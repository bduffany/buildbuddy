import { useEffect, useState } from "react";
import { io } from "../proto/databuddy_ts_proto";
import { CancelablePromise, PromiseType } from "../../../../app/util/async";
import { HTTPStatusError } from "../../../../app/util/errors";

type ErrorResponse = {
  error: string;
};

class APIError extends HTTPStatusError {
  response: ErrorResponse;

  constructor(code: number, rawResponseBody: string, response: ErrorResponse) {
    super(code, rawResponseBody);
    this.response = response;
  }

  toString() {
    return `${this.response.error}`;
  }
}

function isErrorResponse(value: any): value is ErrorResponse {
  return typeof value === "object" && "error" in value && typeof value.error === "string";
}

function fetchFromAPI<T>(path: string): CancelablePromise<T> {
  const abort = new AbortController();
  return new CancelablePromise(
    fetch(`/api/${path}`, { signal: abort.signal })
      .then(async (response) => {
        if (!response.ok) {
          const responseText = await response.text();
          if (!responseText.startsWith("{")) {
            throw new Error(responseText);
          }
          const errorResponse = JSON.parse(responseText);
          if (isErrorResponse(errorResponse)) {
            console.log("response", response);
            throw new APIError(response.status, responseText, errorResponse);
          } else {
            throw new Error("malformed response from server");
          }
        }
        return response.json() as Promise<T>;
      })
      .then((value: T) => value as T),
    { oncancelled: () => abort.abort() }
  );
}

// TODO: better error type
type FetchError = any;

export function useAPI<F extends (...args: any) => CancelablePromise<any>, R = PromiseType<ReturnType<F>>>(
  fn: F | undefined,
  ...args: Parameters<F>
): { response?: R; error?: FetchError; loading?: boolean } {
  const [response, setResponse] = useState<R | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!fn) return;

    const op = fn(...args)
      .then((response) => {
        setResponse(response);
      })
      .catch((e) => setError(e));
    return () => {
      op.cancel();
    };
  }, [fn, ...args]);

  if (!fn) {
    return {};
  }

  if (error) {
    return { error };
  } else if (response) {
    return { response };
  } else {
    return { loading: true };
  }
}

// TODO: auto-generate these functions below

export function getQueries(): CancelablePromise<io.buildbuddy.databuddy.IQueriesResponse> {
  return fetchFromAPI(`queries`);
}

export function getQuery(queryId: string): CancelablePromise<io.buildbuddy.databuddy.IGetQueryResponse> {
  return fetchFromAPI(`query/${queryId}`);
}

export function saveQuery(
  queryId: string,
  content: string
): CancelablePromise<io.buildbuddy.databuddy.ISaveQueryResponse> {
  return fetchFromAPI(`/save/${queryId}?query=${encodeURIComponent(content)}`);
}

export function executeQuery(
  queryId: string,
  content: string
): CancelablePromise<io.buildbuddy.databuddy.IExecuteQueryResponse> {
  return fetchFromAPI(`/execute/${queryId}?query=${encodeURIComponent(content)}`);
}
