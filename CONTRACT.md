# LocalAI provider contract

This provider serves LocalAI through its local OpenAI-compatible chat API and OA's `abstraction.inference/chat@1` leaf boundary.

Binds: [inference.thrift](https://github.com/openabstractions/abstraction-inference/blob/main/inference.thrift)

The conventions every OA contract carries are stated once in [CONVENTIONS.md](https://github.com/openabstractions/abstractions/blob/main/idl/CONVENTIONS.md); a rule here that applies one cites its `CONV-` id.

## [LCL-A1] Local process identity

The operator MUST select the process that listens on the configured IPv4 loopback endpoint and supply its PID and start time. The adapter MUST verify the listener generation before sending request bytes and while reading a response. It MUST refuse a stale or recycled process identity. On platforms that cannot bind loopback connections to their listening process, the adapter MUST refuse construction. The adapter MUST NOT accept an arbitrary HTTP client or a route that can delegate to hosted providers.

## [LCL-A2] Attach-only lifecycle

The adapter MUST treat the configured LocalAI server as externally managed. It MUST NOT start, stop, or unload that server or its backend children. It MAY publish a launch recipe whose command line and environment meet LCL-A5. LocalAI owns its internal process and model lifecycle. If LocalAI exposes a documented safe drain or unload operation, an adapter MAY mediate OA resource-yield requests through it after that behavior has been qualified.

## [LCL-A6] Declared exception to the card [lease](https://openabstractions.github.io/reference.html#concept-resource.lease)

This provider is a declared exception to the rule that every local engine loads under an `abstraction-resource` card lease and the `gpufault` Guard (VISION 2026-10-01, MODELHOST-H1). Under LCL-A2 LocalAI loads, unloads and watches its own engines, so the adapter holds no lease, composes no Guard and declares no `card:<n>` resource. The resource table shows LocalAI's memory through the platform instrument. The exception ends when a qualified drain under LCL-A2 lets the adapter answer a yield.

## [LCL-A3] Explicit models and local placement

The adapter MUST require a nonempty explicit model allowlist and MUST advertise each configured name through a loopback route. It MUST use the shared OA designated leaf for caller identity, admission, and transport. Local placement is established by LCL-A5.

## [LCL-A4] Authorization before backend work

A request denied by OA rights MUST NOT reach LocalAI. A permitted request MUST preserve the configured OA model name in its host and model attribution. The adapter MUST send its per-launch API key with each request and MUST forward only chat completions; gallery, backend-management, settings and every other endpoint MUST be refused before a connection opens.

## [LCL-A5] Reviewed configuration before attach

Before serving, the adapter MUST read the selected listener process's executable image, command line, environment and working directory, and MUST refuse with a typed reason naming the unmet condition unless all of these hold:

1. The executable's SHA-256 equals the reviewed LocalAI release binary.
2. Every address source names `127.0.0.1` on the configured port; there is no socket activation, MITM listener or public-bind override.
3. `LOCALAI_API_KEY` holds exactly the adapter's per-launch key; no key appears on the command line or in `api_keys.json`.
4. Model and backend galleries are empty and do not autoload, backend auto-upgrade is off, and no preload, external backend or model URL is configured.
5. The gallery endpoint is disabled.
6. P2P, federation and distributed mode are off.
7. `LLAMACPP_GRPC_SERVERS`, external gRPC backends, `external_backends.json` and model options that name RPC servers are absent.
8. The environment holds no download token, proxy, dynamic-loader or shell-startup variable; LocalAI's dotenv and credential files and model `env:` blocks are absent.
9. The backends path holds only the reviewed backend, each file matching its recorded hash, and the system backends path is empty or absent.
10. Every model configuration names a local GGUF file whose SHA-256 the adapter's configuration pins, on the reviewed backend, and uses only the keys and options of the reviewed configuration.
11. Every flag and LocalAI setting present is one the adapter evaluates, and no runtime settings file overrides them.

A setting read from several sources MUST be safe in every source. Paths MUST resolve in the process's own root. A process the adapter cannot read MUST be refused. The adapter MUST confirm that the same listener generation holds the endpoint after reading the process. A new LocalAI version, backend digest or backend set requires a new review and new pins.
