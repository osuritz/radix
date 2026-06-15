/**
 * TypeScript interface mirroring the /_metrics JSON contract from
 * internal/metrics/collector.go — Radix v0.7.x.
 */

export interface ServerMetrics {
  /** Active command name: "mock" | "proxy" | "echo" | "serve" */
  command: 'mock' | 'proxy' | 'echo' | 'serve';
  uptime_seconds: number;
  start_time: string; // ISO 8601
  version: string;
}

export interface RequestMetrics {
  total: number;
  success: number;
  errors: number;
  rate_per_second: number;
}

/**
 * Keys are human-readable HTTP status text strings (e.g. "OK", "Not Found",
 * "Internal Server Error"), NOT numeric codes. Values are counts.
 */
export type StatusCodesMap = Record<string, number>;

/**
 * Keys are HTTP method strings (e.g. "GET", "POST"). Values are counts.
 */
export type MethodsMap = Record<string, number>;

export interface ResponseTimesMetrics {
  min_ms: number;
  max_ms: number;
  avg_ms: number;
  p50_ms: number;
  p95_ms: number;
  p99_ms: number;
  count: number;
}

export interface BandwidthMetrics {
  bytes_sent: number;
  bytes_received: number;
  /** omitempty in Go — may be absent when no requests have been processed */
  avg_request_size_bytes?: number;
  /** omitempty in Go — may be absent when no requests have been processed */
  avg_response_size_bytes?: number;
}

export interface EchoMetrics {
  delays_applied: number;
  custom_body_responses: number;
  path_status_hits: number;
}

export interface MockMetrics {
  route_matches_builtin: number;
  route_matches_custom: number;
  template_renders: number;
  template_errors: number;
  reloads: number;
  fail_injections: number;
  fallback_not_found: number;
  fallback_proxy: number;
}

export interface ProxyMetrics {
  auth_injections: number;
  stream_connections: number;
}

/**
 * Per-command counters section. At most ONE of echo/mock/proxy is non-null,
 * matching the active command. "serve" has no per-command counters, so the
 * entire "command" field is omitted (null) from the snapshot.
 */
export interface CommandMetrics {
  echo?: EchoMetrics;
  mock?: MockMetrics;
  proxy?: ProxyMetrics;
}

/**
 * Top-level /_metrics JSON response shape.
 */
export interface MetricsSnapshot {
  server: ServerMetrics;
  requests: RequestMetrics;
  /** Human-readable status text keys (e.g. "OK", "Not Found") */
  status_codes: StatusCodesMap;
  /** HTTP method keys (e.g. "GET", "POST") */
  methods: MethodsMap;
  response_times: ResponseTimesMetrics;
  bandwidth: BandwidthMetrics;
  /**
   * Omitted (undefined) for "serve" command — Go serialises this field with
   * omitempty on a pointer, so it is absent from the JSON, not null.
   * Present for echo/mock/proxy; exactly one sub-field is populated.
   */
  command?: CommandMetrics | null;
}
