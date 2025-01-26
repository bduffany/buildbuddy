-- name: ExecutionsView
-- author: brandon@buildbuddy.io
-- macro: true

SELECT
  *,
  -- Durations from timestamps
  worker_completed_timestamp_usec - worker_start_timestamp_usec AS worker_duration_usec,
  input_fetch_completed_timestamp_usec - input_fetch_start_timestamp_usec AS input_fetch_duration_usec,
  execution_completed_timestamp_usec - execution_start_timestamp_usec AS execution_duration_usec,
  output_upload_completed_timestamp_usec - output_upload_start_timestamp_usec AS output_upload_duration_usec,

  -- Alternate units
  worker_duration_usec / (60e6) AS build_minutes

FROM
  Executions
;