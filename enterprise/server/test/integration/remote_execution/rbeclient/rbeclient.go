package rbeclient

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/buildbuddy-io/buildbuddy/enterprise/server/remote_execution/dirtools"
	"github.com/buildbuddy-io/buildbuddy/server/environment"
	"github.com/buildbuddy-io/buildbuddy/server/remote_cache/cachetools"
	"github.com/buildbuddy-io/buildbuddy/server/remote_cache/digest"
	"github.com/buildbuddy-io/buildbuddy/server/util/log"
	"github.com/buildbuddy-io/buildbuddy/server/util/status"
	"github.com/golang/protobuf/ptypes"

	repb "github.com/buildbuddy-io/buildbuddy/proto/remote_execution"
	bspb "google.golang.org/genproto/googleapis/bytestream"
	gstatus "google.golang.org/grpc/status"
)

type GRPCClientSource interface {
	GetRemoteExecutionClient() repb.ExecutionClient
	GetByteStreamClient() bspb.ByteStreamClient
}

type Client struct {
	gRPClientSource GRPCClientSource
}

func New(gRPCClientSource GRPCClientSource) *Client {
	return &Client{
		gRPClientSource: gRPCClientSource,
	}
}

// LocalStats tracks execution stats from the client's perspective.
type LocalStats struct {
	// Time to issue Execute RPC to server.
	ExecuteRPCStarted time.Duration
	// Time for the server to accept the execution (i.e. how long before we receive the first update from the server)
	TimeToAccepted time.Duration
	// Time for the execution to go from being accepted to being finished (this includes queuing & execution time).
	AcceptedToFinished time.Duration
	// Overall duration, from issuing Execute RPC to receiving the completion response.
	Total time.Duration
}

// CommandResult is the result of a remotely executed command.
type CommandResult struct {
	CommandName  string
	InstanceName string
	Stage        repb.ExecutionStage_Value

	// Execution outcome.
	ID           string
	Err          error
	Executor     string
	ExitCode     int
	ActionResult *repb.ActionResult

	// Local & remote stats.
	LocalStats  LocalStats
	RemoteStats *repb.ExecutedActionMetadata
}

func (r *CommandResult) String() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	return fmt.Sprintf("exit code %d", r.ExitCode)
}

// Command is a handle for a remotely executed command.
type Command struct {
	// Local name to aid debugging.
	Name string

	gRPCClientSource GRPCClientSource

	actionDigest *digest.InstanceNameDigest

	cancelExecutionRequest context.CancelFunc
	accepted               chan string
	status                 chan *CommandResult

	beforeExecuteTime time.Time
	afterExecuteTime  time.Time

	mu     sync.Mutex
	opName string
}

func (c *Command) StatusChannel() <-chan *CommandResult {
	return c.status
}

func (c *Command) AcceptedChannel() <-chan string {
	return c.accepted
}

func (c *Command) Start(ctx context.Context) error {
	executionClient := c.gRPCClientSource.GetRemoteExecutionClient()
	req := &repb.ExecuteRequest{
		InstanceName:    c.actionDigest.GetInstanceName(),
		ActionDigest:    c.actionDigest.Digest,
		SkipCacheLookup: true,
	}

	log.Debugf("Executing command %q with action digest %s", c.Name, c.actionDigest.GetHash())

	beforeExecuteTime := time.Now()
	ctx, cancel := context.WithCancel(ctx)
	stream, err := executionClient.Execute(ctx, req)
	if err != nil {
		cancel()
		return status.UnknownErrorf("unable to request action execution for command %q: %s", c.Name, err)
	}
	afterExecuteTime := time.Now()

	c.cancelExecutionRequest = cancel
	c.beforeExecuteTime = beforeExecuteTime
	c.afterExecuteTime = afterExecuteTime

	// start reading the stream for updates
	c.processUpdates(stream)

	return nil
}

func (c *Command) ReplaceWaitUsingWaitExecutionAPI(ctx context.Context) error {
	if c.opName == "" {
		return status.FailedPreconditionErrorf("Operation name for command is not known. Did you wait for command to be accepted?")
	}

	c.cancelExecutionRequest()

	executionClient := c.gRPCClientSource.GetRemoteExecutionClient()

	log.Debugf("Sending WaitExecution request for command %q using operation name %q", c.Name, c.opName)

	req := &repb.WaitExecutionRequest{
		Name: c.opName,
	}
	stream, err := executionClient.WaitExecution(ctx, req)
	if err != nil {
		return status.UnavailableErrorf("unable to request WaitExecution for command %q using operation %q: %v", c.Name, c.opName, err)
	}
	c.processUpdates(stream)
	return nil
}

// processUpdates starts a goroutine that processes execution updates from the passed stream.
func (c *Command) processUpdates(stream repb.Execution_ExecuteClient) {
	c.status = make(chan *CommandResult, 1)
	c.accepted = make(chan string, 1)
	go func() {
		c.processUpdatesAsync(stream, c.Name, c.status, c.accepted)
	}()
}

// processUpdatesAsync processes execution updates from the stream and publishes execution state updates via the status
// and accepted channels. The accepted channel will receive the name of the operation ID as soon as it's known and the
// status channel will receive progress updates for the execution.
func (c *Command) processUpdatesAsync(stream repb.Execution_ExecuteClient, name string, statusChannel chan *CommandResult, accepted chan string) {
	sendStatus := func(status *CommandResult) {
		c.mu.Lock()
		status.ID = c.opName
		c.mu.Unlock()
		status.CommandName = name
		statusChannel <- status
		if status.Stage == repb.ExecutionStage_COMPLETED {
			log.Debugf("Command [%s] finished: [%s]", name, status)
			close(statusChannel)
		}
	}

	acceptedTime := time.Time{}
	for {
		op, err := stream.Recv()
		if err != nil {
			sendStatus(&CommandResult{
				Stage: repb.ExecutionStage_COMPLETED,
				Err:   status.AbortedErrorf("stream to server broken: %v", err)})
			return
		}

		metadata := &repb.ExecuteOperationMetadata{}
		err = ptypes.UnmarshalAny(op.GetMetadata(), metadata)
		if err != nil {
			sendStatus(&CommandResult{
				Stage: repb.ExecutionStage_COMPLETED,
				Err:   status.InternalErrorf("invalid metadata proto: %s", err)})
			return
		}

		if acceptedTime.IsZero() {
			acceptedTime = time.Now()
			log.Debugf("Command %q accepted by the server as %q", c.Name, op.GetName())
			c.mu.Lock()
			c.opName = op.GetName()
			c.mu.Unlock()
			accepted <- op.GetName()
			close(accepted)
		}

		if !op.GetDone() {
			sendStatus(&CommandResult{Stage: metadata.GetStage()})
			continue
		}

		finishedTime := time.Now()

		response := &repb.ExecuteResponse{}
		err = ptypes.UnmarshalAny(op.GetResponse(), response)
		if err != nil {
			sendStatus(&CommandResult{
				Stage: repb.ExecutionStage_COMPLETED,
				Err:   status.InternalErrorf("invalid response proto: %v", err)})
			return
		}
		err = gstatus.ErrorProto(response.GetStatus())
		if err != nil {
			sendStatus(&CommandResult{
				Stage: repb.ExecutionStage_COMPLETED,
				Err:   status.InternalErrorf("command execution failed: %v", err)})
			return
		}

		res := &CommandResult{
			Stage:        repb.ExecutionStage_COMPLETED,
			Executor:     response.GetResult().GetExecutionMetadata().GetWorker(),
			ExitCode:     int(response.GetResult().GetExitCode()),
			InstanceName: c.actionDigest.GetInstanceName(),
			ActionResult: response.GetResult(),
			LocalStats: LocalStats{
				ExecuteRPCStarted:  c.afterExecuteTime.Sub(c.beforeExecuteTime),
				TimeToAccepted:     acceptedTime.Sub(c.beforeExecuteTime),
				AcceptedToFinished: finishedTime.Sub(acceptedTime),
				Total:              finishedTime.Sub(c.beforeExecuteTime),
			},
			RemoteStats: response.GetResult().GetExecutionMetadata(),
		}
		sendStatus(res)
		return
	}
}

func (c *Client) PrepareCommand(ctx context.Context, instanceName string, name string, inputRootDigest *repb.Digest, commandProto *repb.Command) (*Command, error) {
	commandDigest, err := cachetools.UploadProto(ctx, c.gRPClientSource.GetByteStreamClient(), instanceName, commandProto)
	if err != nil {
		return nil, status.UnknownErrorf("unable to upload command %q to CAS: %s", name, err)
	}

	action := &repb.Action{
		CommandDigest:   commandDigest,
		InputRootDigest: inputRootDigest,
	}
	actionDigest, err := cachetools.UploadProto(ctx, c.gRPClientSource.GetByteStreamClient(), instanceName, action)
	if err != nil {
		return nil, status.UnknownErrorf("unable to upload action for command %q to CAS: %s", name, err)
	}

	command := &Command{
		gRPCClientSource: c.gRPClientSource,
		Name:             name,
		actionDigest:     digest.NewInstanceNameDigest(actionDigest, instanceName),
	}

	return command, nil
}

func (c *Client) GetStdoutAndStderr(ctx context.Context, res *CommandResult) (string, string, error) {
	stdout := ""
	if res.ActionResult.GetStdoutDigest() != nil {
		d := digest.NewInstanceNameDigest(res.ActionResult.GetStdoutDigest(), res.InstanceName)
		buf := bytes.NewBuffer(make([]byte, 0, d.GetSizeBytes()))
		err := cachetools.GetBlob(ctx, c.gRPClientSource.GetByteStreamClient(), d, buf)
		if err != nil {
			return "", "", status.UnavailableErrorf("error retrieving stdout from CAS: %v", err)
		}
		stdout = buf.String()
	}

	stderr := ""
	if res.ActionResult.GetStderrDigest() != nil {
		d := digest.NewInstanceNameDigest(res.ActionResult.GetStderrDigest(), res.InstanceName)
		buf := bytes.NewBuffer(make([]byte, 0, d.GetSizeBytes()))
		err := cachetools.GetBlob(ctx, c.gRPClientSource.GetByteStreamClient(), d, buf)
		if err != nil {
			return "", "", status.InternalErrorf("error retrieving stderr from CAS: %v", err)
		}
		stderr = buf.String()
	}

	return stdout, stderr, nil
}

func (c *Client) DownloadActionOutputs(ctx context.Context, env environment.Env, res *CommandResult, rootDir string) error {
	for _, out := range res.ActionResult.OutputFiles {
		path := filepath.Join(rootDir, out.GetPath())
		if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return err
		}
		d := digest.NewInstanceNameDigest(out.GetDigest(), res.InstanceName)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := cachetools.GetBlob(ctx, c.gRPClientSource.GetByteStreamClient(), d, f); err != nil {
			return err
		}
	}

	for _, dir := range res.ActionResult.OutputDirectories {
		path := filepath.Join(rootDir, dir.GetPath())
		if err := os.MkdirAll(path, 0777); err != nil {
			return err
		}
		treeDigest := digest.NewInstanceNameDigest(dir.GetTreeDigest(), res.InstanceName)
		tree := &repb.Tree{}
		if err := cachetools.GetBlobAsProto(ctx, c.gRPClientSource.GetByteStreamClient(), treeDigest, tree); err != nil {
			return err
		}
		if _, err := dirtools.GetTree(ctx, env, res.InstanceName, tree, path, &dirtools.GetTreeOpts{}); err != nil {
			return err
		}
	}

	// TODO: Download symlinks

	return nil
}