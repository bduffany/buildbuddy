import React from "react";
import { from, Subscription } from "rxjs";
import { User } from "../../../app/auth/auth_service";
import Button, {
  OutlinedButton,
} from "../../../app/components/button/button";
import router from "../../../app/router/router";
import rpcService from "../../../app/service/rpc_service";
import { BuildBuddyError } from "../../../app/util/errors";
import { workflow } from "../../../proto/workflow_ts_proto";
import CreateWorkflowComponent from "./create_workflow";
import WorkflowsZeroStateAnimation from "./zero_state";
import Popup from "../../../app/components/popup/popup";
import Menu, { MenuItem } from "../../../app/components/menu/menu";
import Dialog, {
  DialogBody,
  DialogFooter,
  DialogFooterButtons,
  DialogHeader,
  DialogTitle,
} from "../../../app/components/dialog/dialog";
import Modal from "../../../app/components/modal/modal";
import { copyToClipboard } from "../../../app/util/clipboard";

type Workflow = workflow.GetWorkflowsResponse.IWorkflow;

export type WorkflowsProps = {
  path: string;
  user: User;
};

export default class WorkflowsComponent extends React.Component<WorkflowsProps> {
  render() {
    const { path, user } = this.props;

    if (path.startsWith("/workflows/new")) {
      return <CreateWorkflowComponent user={user} />;
    }

    return <ListWorkflowsComponent user={user} />;
  }
}

type State = {
  error?: BuildBuddyError;
  response?: workflow.GetWorkflowsResponse;
  workflowToDelete?: Workflow | null;
  isDeleting?: boolean;
  deleteError?: BuildBuddyError;
};

export type ListWorkflowsProps = {
  user: User;
};

class ListWorkflowsComponent extends React.Component<ListWorkflowsProps, State> {
  private workflowsSubscription: Subscription = new Subscription();
  state: State = {};

  componentDidMount() {
    document.title = "Workflows | BuildBuddy";
    this.fetchWorkflows();
  }

  componentDidUpdate(prevProps: WorkflowsProps) {
    if (this.props.user !== prevProps.user) {
      this.workflowsSubscription.unsubscribe();
      this.fetchWorkflows();
    }
  }

  componentWillUnmount() {
    this.workflowsSubscription.unsubscribe();
  }

  private fetchWorkflows() {
    if (!this.props.user) return;

    this.state = {};
    this.workflowsSubscription = from<Promise<workflow.GetWorkflowsResponse>>(
      rpcService.service.getWorkflows(new workflow.GetWorkflowsRequest())
    ).subscribe(
      (response) => this.setState({ response }),
      (e) => this.setState({ error: BuildBuddyError.parse(e) })
    );
  }

  private onClickCreate() {
    router.navigateTo("/workflows/new");
  }

  private onClickDeleteItem(workflowToDelete: Workflow) {
    this.setState({ workflowToDelete });
  }

  private async onClickDelete() {
    try {
      await rpcService.service.deleteWorkflow(
        new workflow.DeleteWorkflowRequest({ id: this.state.workflowToDelete.id })
      );
      this.setState({ workflowToDelete: null });

      this.workflowsSubscription.unsubscribe();
      this.fetchWorkflows();
    } catch (e) {
      this.setState({ deleteError: BuildBuddyError.parse(e) });
    } finally {
      this.setState({ isDeleting: false });
    }
  }

  private onCloseDeleteDialog() {
    this.setState({ workflowToDelete: null, deleteError: null });
  }

  render() {
    const { error, response, workflowToDelete, isDeleting, deleteError } = this.state;
    const loading = !(error || response);
    if (loading) {
      return <div className="loading" />;
    }

    const workflowToDeleteUrl = workflowToDelete ? new URL(workflowToDelete.repoUrl) : null;

    return (
      <div className="workflows-page">
        <div className="shelf">
          <div className="container">
            {/* TODO: Breadcrumbs */}
            <div className="title">Workflows</div>
            <div className="create-new-container">
              <Button onClick={this.onClickCreate.bind(this)}>Add repository</Button>
            </div>
          </div>
        </div>
        <div className="content">
          {/* TODO: better styling of this error */}
          {error && <div className="error">{error.message}</div>}
          {response && !response.workflow.length && (
            <div className="no-workflows-container">
              <div className="no-workflows-card">
                <WorkflowsZeroStateAnimation />
                <div className="details">
                  <div>
                    Workflows automatically build and test your code with BuildBuddy when commits are pushed to your
                    repo.
                  </div>
                  <Button onClick={this.onClickCreate.bind(this)}>Create your first workflow</Button>
                </div>
              </div>
            </div>
          )}
          {Boolean(response?.workflow?.length) && (
            <>
              <div className="workflows-list">
                {response.workflow.map((workflow) => (
                  <WorkflowItem workflow={workflow} onClickDeleteItem={this.onClickDeleteItem.bind(this)} />
                ))}
              </div>
              <Modal isOpen={Boolean(workflowToDelete)} onRequestClose={this.onCloseDeleteDialog.bind(this)}>
                <Dialog className="delete-workflow-dialog">
                  <DialogHeader>
                    <DialogTitle>Delete workflow</DialogTitle>
                  </DialogHeader>
                  <DialogBody className="dialog-body">
                    <div>
                      Are you sure you want to unlink{" "}
                      <strong>
                        {workflowToDeleteUrl?.host}
                        {workflowToDeleteUrl?.pathname}
                      </strong>
                      ? This will prevent BuildBuddy workflows from being run.
                    </div>
                    {deleteError && <div className="error">{deleteError.message}</div>}
                  </DialogBody>
                  <DialogFooter>
                    <DialogFooterButtons>
                      {this.state.isDeleting && <div className="loading" />}
                      <Button className="destructive" onClick={this.onClickDelete.bind(this)} disabled={isDeleting}>
                        Delete
                      </Button>
                    </DialogFooterButtons>
                  </DialogFooter>
                </Dialog>
              </Modal>
            </>
          )}
        </div>
      </div>
    );
  }
}

type WorkflowItemProps = {
  workflow: Workflow;
  onClickDeleteItem: (workflow: Workflow) => void;
};

type WorkflowItemState = {
  isMenuOpen?: boolean;
  copiedToClipboard?: boolean;
};

class WorkflowItem extends React.Component<WorkflowItemProps, WorkflowItemState> {
  state: WorkflowItemState = {};

  private onClickMenuButton() {
    this.setState({ isMenuOpen: !this.state.isMenuOpen });
  }

  private onCloseMenu() {
    this.setState({ isMenuOpen: false });
  }

  private onClickCopyWebhookUrl() {
    copyToClipboard(this.props.workflow.webhookUrl);
    this.setState({ copiedToClipboard: true });
    setTimeout(() => {
      this.setState({ copiedToClipboard: false });
    }, 1000);
  }

  private onClickDeleteMenuItem() {
    this.setState({ isMenuOpen: false });
    this.props.onClickDeleteItem(this.props.workflow);
  }

  render() {
    const { name, repoUrl } = this.props.workflow;
    const { isMenuOpen, copiedToClipboard } = this.state;

    const url = new URL(repoUrl);
    url.protocol = "https:";

    return (
      <div className="workflow-item container">
        <div className="workflow-item-column">
          <div className="workflow-item-row">
            <img className="git-merge-icon" src="/image/git-merge.svg" alt="" />
            <a href={url.toString()} className="repo-url" target="_new">
              {url.host}
              {url.pathname}
            </a>
          </div>
        </div>
        <div className="workflow-item-column workflow-dropdown-container">
          <div>
            <OutlinedButton
              title="Workflow options"
              className="workflow-dropdown-button"
              onClick={this.onClickMenuButton.bind(this)}>
              <img src="/image/more-vertical.svg" alt="" />
            </OutlinedButton>
            <Popup isOpen={isMenuOpen} onRequestClose={this.onCloseMenu.bind(this)}>
              <Menu className="workflow-dropdown-menu">
                <MenuItem
                  onClick={this.onClickCopyWebhookUrl.bind(this)}
                  className={copiedToClipboard ? "copied-to-clipboard" : ""}>
                  {copiedToClipboard ? <>Copied!</> : <>Copy webhook URL</>}
                </MenuItem>
                <MenuItem onClick={this.onClickDeleteMenuItem.bind(this)}>Delete workflow</MenuItem>
              </Menu>
            </Popup>
          </div>
        </div>
      </div>
    );
  }
}