package localai

import (
	"path"
	"strconv"
	"strings"
)

// LocalAI v4.10.0 reads its server settings through kong from flags and from
// environment variables (core/cli/run.go and core/cli/context/context.go at
// tag commit 7ad0cbf259f0c7bf9920fe2438fc3630ecd6c672). A flag overrides the
// environment and several variables alias one setting, so the check requires
// every source of a setting to be safe, independent of precedence. Flag names
// are kong v1.15.0's derivation from the field names: the name tag when
// present, otherwise strings.ToLower(strings.Join(camelCase(field), "-"))
// (kong.go:93, build.go:45), plus aliases tags. Where the CLI reference
// document differs (--disable-webui, --github-client-id), kong's name is the
// one LocalAI parses.

type policy int

const (
	// polNeutral settings do not affect the reviewed conditions.
	polNeutral policy = iota
	// polPath settings name a directory the check inspects; every source must agree.
	polPath
	// polAddress requires 127.0.0.1 on OA's port from every source, and at
	// least one source.
	polAddress
	// polAPIKey requires OA's key from the environment only.
	polAPIKey
	// polEmptyList requires the JSON list "[]" from at least one source.
	polEmptyList
	// polMustTrue requires true from at least one source (default false).
	polMustTrue
	// polMustFalse requires false from at least one source (default true).
	polMustFalse
	// polFalseIfSet allows only false (default false).
	polFalseIfSet
	// polAbsent refuses any source.
	polAbsent
)

type setting struct {
	field   string
	flags   []string
	envs    []string
	boolean bool
	pol     policy
	cond    Condition
}

func s(field, flags, envs string, boolean bool, pol policy, cond Condition) setting {
	split := func(v string) []string {
		if v == "" {
			return nil
		}
		return strings.Split(v, ",")
	}
	return setting{field: field, flags: split(flags), envs: split(envs), boolean: boolean, pol: pol, cond: cond}
}

const (
	isBool = true
	notB   = false
	known  = ConditionKnownConfiguration
)

// settings is every RunCMD and Context field of LocalAI v4.10.0 that has an
// environment variable. A field without an evaluated policy refuses any
// source: OA does not know what it changes.
var settings = []setting{
	// Listener and authentication.
	s("Address", "address", "LOCALAI_ADDRESS,ADDRESS", notB, polAddress, ConditionLoopbackListener),
	s("APIKeys", "api-keys", "LOCALAI_API_KEY,API_KEY", notB, polAPIKey, ConditionAPIKey),
	s("AllowInsecurePublicBind", "allow-insecure-public-bind", "LOCALAI_ALLOW_INSECURE_PUBLIC_BIND", isBool, polFalseIfSet, ConditionLoopbackListener),
	s("MITMListen", "mitm-listen", "LOCALAI_MITM_LISTEN", notB, polAbsent, ConditionLoopbackListener),
	s("MITMCADir", "mitmca-dir", "LOCALAI_MITM_CA_DIR", notB, polAbsent, ConditionLoopbackListener),
	s("DisableApiKeyRequirementForHttpGet", "disable-api-key-requirement-for-http-get", "LOCALAI_DISABLE_API_KEY_REQUIREMENT_FOR_HTTP_GET", isBool, polFalseIfSet, ConditionAPIKey),
	s("HttpGetExemptedEndpoints", "http-get-exempted-endpoints", "LOCALAI_HTTP_GET_EXEMPTED_ENDPOINTS", notB, polAbsent, ConditionAPIKey),

	// Galleries and start-time installs.
	s("Galleries", "galleries", "LOCALAI_GALLERIES,GALLERIES", notB, polEmptyList, ConditionGalleriesOff),
	s("BackendGalleries", "backend-galleries", "LOCALAI_BACKEND_GALLERIES,BACKEND_GALLERIES", notB, polEmptyList, ConditionGalleriesOff),
	s("AutoloadGalleries", "autoload-galleries", "LOCALAI_AUTOLOAD_GALLERIES,AUTOLOAD_GALLERIES", isBool, polMustFalse, ConditionGalleriesOff),
	s("AutoloadBackendGalleries", "autoload-backend-galleries", "LOCALAI_AUTOLOAD_BACKEND_GALLERIES,AUTOLOAD_BACKEND_GALLERIES", isBool, polMustFalse, ConditionGalleriesOff),
	s("AutoUpgradeBackends", "auto-upgrade-backends", "LOCALAI_AUTO_UPGRADE_BACKENDS,AUTO_UPGRADE_BACKENDS", isBool, polFalseIfSet, ConditionGalleriesOff),
	s("ExternalBackends", "external-backends", "LOCALAI_EXTERNAL_BACKENDS,EXTERNAL_BACKENDS", notB, polAbsent, ConditionGalleriesOff),
	s("PreloadModels", "preload-models", "LOCALAI_PRELOAD_MODELS,PRELOAD_MODELS", notB, polAbsent, ConditionGalleriesOff),
	s("PreloadModelsConfig", "preload-models-config", "LOCALAI_PRELOAD_MODELS_CONFIG,PRELOAD_MODELS_CONFIG", notB, polAbsent, ConditionGalleriesOff),
	s("BackendImagesReleaseTag", "backend-images-release-tag", "LOCALAI_BACKEND_IMAGES_RELEASE_TAG,BACKEND_IMAGES_RELEASE_TAG", notB, polAbsent, ConditionGalleriesOff),
	s("BackendImagesBranchTag", "backend-images-branch-tag", "LOCALAI_BACKEND_IMAGES_BRANCH_TAG,BACKEND_IMAGES_BRANCH_TAG", notB, polAbsent, ConditionGalleriesOff),
	s("BackendDevSuffix", "backend-dev-suffix", "LOCALAI_BACKEND_DEV_SUFFIX,BACKEND_DEV_SUFFIX", notB, polAbsent, ConditionGalleriesOff),
	s("PreferDevelopmentBackends", "prefer-development-backends", "LOCALAI_PREFER_DEV_BACKENDS,PREFER_DEV_BACKENDS", isBool, polAbsent, ConditionGalleriesOff),
	s("ArtifactDownloadConcurrency", "artifact-download-concurrency", "LOCALAI_ARTIFACT_DOWNLOAD_CONCURRENCY", notB, polAbsent, ConditionGalleriesOff),
	s("DisableGalleryEndpoint", "disable-gallery-endpoint", "LOCALAI_DISABLE_GALLERY_ENDPOINT,DISABLE_GALLERY_ENDPOINT", isBool, polMustTrue, ConditionGalleryEndpointOff),

	// Peer networks.
	s("Peer2Peer", "p2p", "LOCALAI_P2P,P2P", isBool, polFalseIfSet, ConditionNoPeerNetwork),
	s("Peer2PeerDHTInterval", "p2p-dht-interval", "LOCALAI_P2P_DHT_INTERVAL,P2P_DHT_INTERVAL", notB, polAbsent, ConditionNoPeerNetwork),
	s("Peer2PeerOTPInterval", "p2p-otp-interval", "LOCALAI_P2P_OTP_INTERVAL,P2P_OTP_INTERVAL", notB, polAbsent, ConditionNoPeerNetwork),
	s("Peer2PeerToken", "p2p-token,p2ptoken", "LOCALAI_P2P_TOKEN,P2P_TOKEN,TOKEN", notB, polAbsent, ConditionNoPeerNetwork),
	s("Peer2PeerNetworkID", "peer-2-peer-network-id", "LOCALAI_P2P_NETWORK_ID,P2P_NETWORK_ID", notB, polAbsent, ConditionNoPeerNetwork),
	s("Federated", "federated", "LOCALAI_FEDERATED,FEDERATED", isBool, polFalseIfSet, ConditionNoPeerNetwork),
	s("Distributed", "distributed", "LOCALAI_DISTRIBUTED", isBool, polFalseIfSet, ConditionNoPeerNetwork),
	s("NatsURL", "nats-url", "LOCALAI_NATS_URL", notB, polAbsent, ConditionNoPeerNetwork),
	s("MachineTag", "machine-tag", "LOCALAI_MACHINE_TAG,MACHINE_TAG", notB, polAbsent, ConditionNoPeerNetwork),

	// Remote backends.
	s("ExternalGRPCBackends", "external-grpc-backends", "LOCALAI_EXTERNAL_GRPC_BACKENDS,EXTERNAL_GRPC_BACKENDS", notB, polAbsent, ConditionNoRemoteBackends),
	s("PreloadBackendOnly", "preload-backend-only", "LOCALAI_PRELOAD_BACKEND_ONLY,PRELOAD_BACKEND_ONLY", isBool, polFalseIfSet, ConditionNoRemoteBackends),

	// Download credentials.
	s("HFToken", "hf-token", "HF_TOKEN", notB, polAbsent, ConditionCleanEnvironment),
	s("CredentialsFile", "credentials-file", "LOCALAI_CREDENTIALS_FILE", notB, polAbsent, ConditionCleanEnvironment),

	// Models named outside the models directory.
	s("Models", "models", "LOCALAI_MODELS,MODELS", notB, polAbsent, ConditionLocalModels),
	s("ModelsConfigFile", "models-config-file,config-file", "LOCALAI_MODELS_CONFIG_FILE,CONFIG_FILE", notB, polAbsent, ConditionLocalModels),

	// Directories the check inspects.
	s("ModelsPath", "models-path", "LOCALAI_MODELS_PATH,MODELS_PATH", notB, polPath, known),
	s("BackendsPath", "backends-path", "LOCALAI_BACKENDS_PATH,BACKENDS_PATH", notB, polPath, known),
	s("BackendsSystemPath", "backends-system-path", "LOCALAI_BACKENDS_SYSTEM_PATH,BACKEND_SYSTEM_PATH", notB, polPath, known),
	s("DataPath", "data-path", "LOCALAI_DATA_PATH", notB, polPath, known),
	s("LocalaiConfigDir", "localai-config-dir", "LOCALAI_CONFIG_DIR", notB, polPath, known),
	s("GeneratedContentPath", "generated-content-path", "LOCALAI_GENERATED_CONTENT_PATH,GENERATED_CONTENT_PATH", notB, polPath, known),
	s("UploadPath", "upload-path", "LOCALAI_UPLOAD_PATH,UPLOAD_PATH", notB, polPath, known),

	// Settings without effect on the reviewed conditions.
	s("Color", "color", "COLOR", notB, polNeutral, known),
	s("NoColor", "no-color", "NO_COLOR", notB, polNeutral, known),
	s("Debug", "debug", "LOCALAI_DEBUG,DEBUG", isBool, polNeutral, known),
	s("LogLevel", "log-level", "LOCALAI_LOG_LEVEL", notB, polNeutral, known),
	s("LogFormat", "log-format", "LOCALAI_LOG_FORMAT", notB, polNeutral, known),
	s("LogDedupLogs", "log-dedup-logs,no-log-dedup-logs", "LOCALAI_LOG_DEDUP", isBool, polNeutral, known),
	s("F16", "f16", "LOCALAI_F16,F16", isBool, polNeutral, known),
	s("Threads", "threads", "LOCALAI_THREADS,THREADS", notB, polNeutral, known),
	s("ContextSize", "context-size", "LOCALAI_CONTEXT_SIZE,CONTEXT_SIZE", notB, polNeutral, known),
	s("UploadLimit", "upload-limit", "LOCALAI_UPLOAD_LIMIT,UPLOAD_LIMIT", notB, polNeutral, known),
	s("DisableWebUI", "disable-web-ui", "LOCALAI_DISABLE_WEBUI,DISABLE_WEBUI", isBool, polNeutral, known),
	s("DisableRuntimeSettings", "disable-runtime-settings", "LOCALAI_DISABLE_RUNTIME_SETTINGS,DISABLE_RUNTIME_SETTINGS", isBool, polNeutral, known),
	s("DisableMetricsEndpoint", "disable-metrics-endpoint", "LOCALAI_DISABLE_METRICS_ENDPOINT,DISABLE_METRICS_ENDPOINT", isBool, polNeutral, known),
	s("DisableHTTPCompression", "disable-http-compression", "LOCALAI_DISABLE_HTTP_COMPRESSION", isBool, polNeutral, known),
	s("HTTPCompressionMinLength", "http-compression-min-length", "LOCALAI_HTTP_COMPRESSION_MIN_LENGTH", notB, polNeutral, known),
	s("OpaqueErrors", "opaque-errors", "LOCALAI_OPAQUE_ERRORS", isBool, polNeutral, known),
	s("UseSubtleKeyComparison", "use-subtle-key-comparison", "LOCALAI_SUBTLE_KEY_COMPARISON", isBool, polNeutral, known),
	s("DisablePredownloadScan", "disable-predownload-scan", "LOCALAI_DISABLE_PREDOWNLOAD_SCAN", isBool, polNeutral, known),
	s("RequireBackendIntegrity", "require-backend-integrity", "LOCALAI_REQUIRE_BACKEND_INTEGRITY,REQUIRE_BACKEND_INTEGRITY", isBool, polNeutral, known),
	s("VRAMPersistentCache", "vram-persistent-cache", "LOCALAI_VRAM_PERSISTENT_CACHE,VRAM_PERSISTENT_CACHE", isBool, polNeutral, known),
	s("LoadToMemory", "load-to-memory", "LOCALAI_LOAD_TO_MEMORY,LOAD_TO_MEMORY", notB, polNeutral, known),
	s("SingleActiveBackend", "single-active-backend", "LOCALAI_SINGLE_ACTIVE_BACKEND,SINGLE_ACTIVE_BACKEND", isBool, polNeutral, known),
	s("MaxActiveBackends", "max-active-backends", "LOCALAI_MAX_ACTIVE_BACKENDS,MAX_ACTIVE_BACKENDS", notB, polNeutral, known),
	s("MaxConcurrentBackendRequests", "max-concurrent-backend-requests", "LOCALAI_MAX_CONCURRENT_BACKEND_REQUESTS,MAX_CONCURRENT_BACKEND_REQUESTS", notB, polNeutral, known),
	s("EnableWatchdogIdle", "enable-watchdog-idle", "LOCALAI_WATCHDOG_IDLE,WATCHDOG_IDLE", isBool, polNeutral, known),
	s("WatchdogIdleTimeout", "watchdog-idle-timeout", "LOCALAI_WATCHDOG_IDLE_TIMEOUT,WATCHDOG_IDLE_TIMEOUT", notB, polNeutral, known),
	s("EnableWatchdogBusy", "enable-watchdog-busy", "LOCALAI_WATCHDOG_BUSY,WATCHDOG_BUSY", isBool, polNeutral, known),
	s("WatchdogBusyTimeout", "watchdog-busy-timeout", "LOCALAI_WATCHDOG_BUSY_TIMEOUT,WATCHDOG_BUSY_TIMEOUT", notB, polNeutral, known),
	s("WatchdogInterval", "watchdog-interval", "LOCALAI_WATCHDOG_INTERVAL,WATCHDOG_INTERVAL", notB, polNeutral, known),
	s("ForceEvictionWhenBusy", "force-eviction-when-busy", "LOCALAI_FORCE_EVICTION_WHEN_BUSY,FORCE_EVICTION_WHEN_BUSY", isBool, polNeutral, known),
	s("SizeAwareEviction", "size-aware-eviction", "LOCALAI_SIZE_AWARE_EVICTION,SIZE_AWARE_EVICTION", isBool, polNeutral, known),
	s("LRUEvictionMaxRetries", "lru-eviction-max-retries", "LOCALAI_LRU_EVICTION_MAX_RETRIES,LRU_EVICTION_MAX_RETRIES", notB, polNeutral, known),
	s("LRUEvictionRetryInterval", "lru-eviction-retry-interval", "LOCALAI_LRU_EVICTION_RETRY_INTERVAL,LRU_EVICTION_RETRY_INTERVAL", notB, polNeutral, known),
	s("ModelLoadFailureCooldown", "model-load-failure-cooldown", "LOCALAI_MODEL_LOAD_FAILURE_COOLDOWN,MODEL_LOAD_FAILURE_COOLDOWN", notB, polNeutral, known),
	s("EnableMemoryReclaimer", "enable-memory-reclaimer", "LOCALAI_MEMORY_RECLAIMER,MEMORY_RECLAIMER,LOCALAI_GPU_RECLAIMER,GPU_RECLAIMER", isBool, polNeutral, known),
	s("MemoryReclaimerThreshold", "memory-reclaimer-threshold", "LOCALAI_MEMORY_RECLAIMER_THRESHOLD,MEMORY_RECLAIMER_THRESHOLD,LOCALAI_GPU_RECLAIMER_THRESHOLD,GPU_RECLAIMER_THRESHOLD", notB, polNeutral, known),
	s("DisableMCP", "disable-mcp", "LOCALAI_DISABLE_MCP,DISABLE_MCP", isBool, polNeutral, known),
	s("DisableAgents", "disable-agents", "LOCALAI_DISABLE_AGENTS", isBool, polNeutral, known),
	s("DisableLocalAIAssistant", "disable-local-ai-assistant", "LOCALAI_DISABLE_ASSISTANT", isBool, polNeutral, known),

	// Defined by LocalAI and not evaluated here: any source refuses.
	s("WebRTCNAT1To1IPs", "web-rtcnat-1-to-1-i-ps", "LOCALAI_WEBRTC_NAT_1TO1_IPS,WEBRTC_NAT_1TO1_IPS", notB, polAbsent, known),
	s("WebRTCICEInterfaces", "web-rtcice-interfaces", "LOCALAI_WEBRTC_ICE_INTERFACES,WEBRTC_ICE_INTERFACES", notB, polAbsent, known),
	s("WebRTCUDPPort", "web-rtc-udp-port", "LOCALAI_WEBRTC_UDP_PORT", notB, polAbsent, known),
	s("LocalaiConfigDirPollInterval", "localai-config-dir-poll-interval", "LOCALAI_CONFIG_DIR_POLL_INTERVAL", notB, polAbsent, known),
	s("CORS", "cors", "LOCALAI_CORS,CORS", isBool, polAbsent, known),
	s("CORSAllowOrigins", "cors-allow-origins", "LOCALAI_CORS_ALLOW_ORIGINS,CORS_ALLOW_ORIGINS", notB, polAbsent, known),
	s("DisableCSRF", "disable-csrf", "LOCALAI_DISABLE_CSRF", isBool, polAbsent, known),
	s("OllamaAPIRootEndpoint", "ollama-api-root-endpoint", "LOCALAI_OLLAMA_API_ROOT_ENDPOINT", isBool, polAbsent, known),
	s("VRAMBudget", "vram-budget", "LOCALAI_VRAM_BUDGET", notB, polAbsent, known),
	s("EnableTracing", "enable-tracing", "LOCALAI_ENABLE_TRACING,ENABLE_TRACING", isBool, polAbsent, known),
	s("TracingMaxItems", "tracing-max-items", "LOCALAI_TRACING_MAX_ITEMS", notB, polAbsent, known),
	s("TracingMaxBodyBytes", "tracing-max-body-bytes", "LOCALAI_TRACING_MAX_BODY_BYTES", notB, polAbsent, known),
	s("AgentJobRetentionDays", "agent-job-retention-days", "LOCALAI_AGENT_JOB_RETENTION_DAYS,AGENT_JOB_RETENTION_DAYS", notB, polAbsent, known),
	s("OpenResponsesStoreTTL", "open-responses-store-ttl", "LOCALAI_OPEN_RESPONSES_STORE_TTL,OPEN_RESPONSES_STORE_TTL", notB, polAbsent, known),
	s("AgentPoolAPIURL", "agent-pool-apiurl", "LOCALAI_AGENT_POOL_API_URL", notB, polAbsent, known),
	s("AgentPoolAPIKey", "agent-pool-api-key", "LOCALAI_AGENT_POOL_API_KEY", notB, polAbsent, known),
	s("AgentPoolDefaultModel", "agent-pool-default-model", "LOCALAI_AGENT_POOL_DEFAULT_MODEL", notB, polAbsent, known),
	s("AgentPoolMultimodalModel", "agent-pool-multimodal-model", "LOCALAI_AGENT_POOL_MULTIMODAL_MODEL", notB, polAbsent, known),
	s("AgentPoolTranscriptionModel", "agent-pool-transcription-model", "LOCALAI_AGENT_POOL_TRANSCRIPTION_MODEL", notB, polAbsent, known),
	s("AgentPoolTranscriptionLanguage", "agent-pool-transcription-language", "LOCALAI_AGENT_POOL_TRANSCRIPTION_LANGUAGE", notB, polAbsent, known),
	s("AgentPoolTTSModel", "agent-pool-tts-model", "LOCALAI_AGENT_POOL_TTS_MODEL", notB, polAbsent, known),
	s("AgentPoolStateDir", "agent-pool-state-dir", "LOCALAI_AGENT_POOL_STATE_DIR", notB, polAbsent, known),
	s("AgentPoolTimeout", "agent-pool-timeout", "LOCALAI_AGENT_POOL_TIMEOUT", notB, polAbsent, known),
	s("AgentPoolEnableSkills", "agent-pool-enable-skills", "LOCALAI_AGENT_POOL_ENABLE_SKILLS", isBool, polAbsent, known),
	s("AgentPoolVectorEngine", "agent-pool-vector-engine", "LOCALAI_AGENT_POOL_VECTOR_ENGINE", notB, polAbsent, known),
	s("AgentPoolEmbeddingModel", "agent-pool-embedding-model", "LOCALAI_AGENT_POOL_EMBEDDING_MODEL", notB, polAbsent, known),
	s("AgentPoolCustomActionsDir", "agent-pool-custom-actions-dir", "LOCALAI_AGENT_POOL_CUSTOM_ACTIONS_DIR", notB, polAbsent, known),
	s("AgentPoolDatabaseURL", "agent-pool-database-url", "LOCALAI_AGENT_POOL_DATABASE_URL", notB, polAbsent, known),
	s("AgentPoolMaxChunkingSize", "agent-pool-max-chunking-size", "LOCALAI_AGENT_POOL_MAX_CHUNKING_SIZE", notB, polAbsent, known),
	s("AgentPoolChunkOverlap", "agent-pool-chunk-overlap", "LOCALAI_AGENT_POOL_CHUNK_OVERLAP", notB, polAbsent, known),
	s("AgentPoolEnableLogs", "agent-pool-enable-logs", "LOCALAI_AGENT_POOL_ENABLE_LOGS", isBool, polAbsent, known),
	s("AgentPoolCollectionDBPath", "agent-pool-collection-db-path", "LOCALAI_AGENT_POOL_COLLECTION_DB_PATH", notB, polAbsent, known),
	s("AgentHubURL", "agent-hub-url", "LOCALAI_AGENT_HUB_URL", notB, polAbsent, known),
	s("AuthEnabled", "auth-enabled", "LOCALAI_AUTH", isBool, polAbsent, known),
	s("AuthDatabaseURL", "auth-database-url", "LOCALAI_AUTH_DATABASE_URL,DATABASE_URL", notB, polAbsent, known),
	s("GitHubClientID", "git-hub-client-id", "GITHUB_CLIENT_ID", notB, polAbsent, known),
	s("GitHubClientSecret", "git-hub-client-secret", "GITHUB_CLIENT_SECRET", notB, polAbsent, known),
	s("OIDCIssuer", "oidc-issuer", "LOCALAI_OIDC_ISSUER", notB, polAbsent, known),
	s("OIDCClientID", "oidc-client-id", "LOCALAI_OIDC_CLIENT_ID", notB, polAbsent, known),
	s("OIDCClientSecret", "oidc-client-secret", "LOCALAI_OIDC_CLIENT_SECRET", notB, polAbsent, known),
	s("ExternalBaseURL", "external-base-url", "LOCALAI_BASE_URL", notB, polAbsent, known),
	s("AuthAdminEmail", "auth-admin-email", "LOCALAI_ADMIN_EMAIL", notB, polAbsent, known),
	s("AuthRegistrationMode", "auth-registration-mode", "LOCALAI_REGISTRATION_MODE", notB, polAbsent, known),
	s("DisableLocalAuth", "disable-local-auth", "LOCALAI_DISABLE_LOCAL_AUTH", isBool, polAbsent, known),
	s("AuthAPIKeyHMACSecret", "auth-api-key-hmac-secret", "LOCALAI_AUTH_HMAC_SECRET", notB, polAbsent, known),
	s("DefaultAPIKeyExpiry", "default-api-key-expiry", "LOCALAI_DEFAULT_API_KEY_EXPIRY", notB, polAbsent, known),
	s("InstanceID", "instance-id", "LOCALAI_INSTANCE_ID", notB, polAbsent, ConditionNoPeerNetwork),
	s("StorageURL", "storage-url", "LOCALAI_STORAGE_URL", notB, polAbsent, ConditionNoPeerNetwork),
	s("StorageBucket", "storage-bucket", "LOCALAI_STORAGE_BUCKET", notB, polAbsent, ConditionNoPeerNetwork),
	s("StorageRegion", "storage-region", "LOCALAI_STORAGE_REGION", notB, polAbsent, ConditionNoPeerNetwork),
	s("StorageAccessKey", "storage-access-key", "LOCALAI_STORAGE_ACCESS_KEY", notB, polAbsent, ConditionNoPeerNetwork),
	s("StorageSecretKey", "storage-secret-key", "LOCALAI_STORAGE_SECRET_KEY", notB, polAbsent, ConditionNoPeerNetwork),
	s("RegistrationToken", "registration-token", "LOCALAI_REGISTRATION_TOKEN", notB, polAbsent, ConditionNoPeerNetwork),
	s("RegistrationRequireAuth", "registration-require-auth", "LOCALAI_REGISTRATION_REQUIRE_AUTH", isBool, polAbsent, ConditionNoPeerNetwork),
	s("DistributedRequireAuth", "distributed-require-auth", "LOCALAI_DISTRIBUTED_REQUIRE_AUTH", isBool, polAbsent, ConditionNoPeerNetwork),
	s("AutoApproveNodes", "auto-approve-nodes", "LOCALAI_AUTO_APPROVE_NODES", isBool, polAbsent, ConditionNoPeerNetwork),
	s("DistributedSharedModels", "distributed-shared-models", "LOCALAI_DISTRIBUTED_SHARED_MODELS", isBool, polAbsent, ConditionNoPeerNetwork),
	s("DistributedPrefixCache", "distributed-prefix-cache", "LOCALAI_DISTRIBUTED_PREFIX_CACHE", isBool, polAbsent, ConditionNoPeerNetwork),
	s("DistributedDiskHeadroomCheck", "distributed-disk-headroom-check", "LOCALAI_DISTRIBUTED_DISK_HEADROOM_CHECK", isBool, polAbsent, ConditionNoPeerNetwork),
	s("DistributedPrefixCacheTTL", "distributed-prefix-cache-ttl", "LOCALAI_DISTRIBUTED_PREFIX_CACHE_TTL", notB, polAbsent, ConditionNoPeerNetwork),
	s("BackendInstallTimeout", "backend-install-timeout", "LOCALAI_NATS_BACKEND_INSTALL_TIMEOUT", notB, polAbsent, ConditionNoPeerNetwork),
	s("BackendUpgradeTimeout", "backend-upgrade-timeout", "LOCALAI_NATS_BACKEND_UPGRADE_TIMEOUT", notB, polAbsent, ConditionNoPeerNetwork),
	s("ModelLoadTimeout", "model-load-timeout", "LOCALAI_NATS_MODEL_LOAD_TIMEOUT", notB, polAbsent, ConditionNoPeerNetwork),
	s("ModelLoadWait", "model-load-wait", "LOCALAI_MODEL_LOAD_WAIT", notB, polAbsent, ConditionNoPeerNetwork),
	s("StaleNodeThreshold", "stale-node-threshold", "LOCALAI_STALE_NODE_THRESHOLD", notB, polAbsent, ConditionNoPeerNetwork),
	s("NodeHeartbeatCheckpoint", "node-heartbeat-checkpoint", "LOCALAI_NODE_HEARTBEAT_CHECKPOINT", notB, polAbsent, ConditionNoPeerNetwork),
	s("NatsAccountSeed", "nats-account-seed", "LOCALAI_NATS_ACCOUNT_SEED", notB, polAbsent, ConditionNoPeerNetwork),
	s("NatsServiceJWT", "nats-service-jwt", "LOCALAI_NATS_SERVICE_JWT", notB, polAbsent, ConditionNoPeerNetwork),
	s("NatsServiceSeed", "nats-service-seed", "LOCALAI_NATS_SERVICE_SEED", notB, polAbsent, ConditionNoPeerNetwork),
	s("NatsWorkerJWTTTL", "nats-worker-jwtttl", "LOCALAI_NATS_WORKER_JWT_TTL", notB, polAbsent, ConditionNoPeerNetwork),
	s("NatsRequireAuth", "nats-require-auth", "LOCALAI_NATS_REQUIRE_AUTH", isBool, polAbsent, ConditionNoPeerNetwork),
	s("NatsTLSCA", "nats-tlsca", "LOCALAI_NATS_TLS_CA", notB, polAbsent, ConditionNoPeerNetwork),
	s("NatsTLSCert", "nats-tls-cert", "LOCALAI_NATS_TLS_CERT", notB, polAbsent, ConditionNoPeerNetwork),
	s("NatsTLSKey", "nats-tls-key", "LOCALAI_NATS_TLS_KEY", notB, polAbsent, ConditionNoPeerNetwork),
	s("ExposeNodeHeader", "expose-node-header", "LOCALAI_EXPOSE_NODE_HEADER", isBool, polAbsent, ConditionNoPeerNetwork),
	s("ModelScheduling", "model-scheduling", "LOCALAI_MODEL_SCHEDULING", notB, polAbsent, ConditionNoPeerNetwork),
	s("ModelSchedulingConfig", "model-scheduling-config", "LOCALAI_MODEL_SCHEDULING_CONFIG", notB, polAbsent, ConditionNoPeerNetwork),
	s("PIIDefaultDetectors", "pii-default-detectors", "LOCALAI_PII_DEFAULT_DETECTORS", notB, polAbsent, known),
}

var (
	settingsByEnv  = map[string]*setting{}
	settingsByFlag = map[string]*setting{}
)

func init() {
	for i := range settings {
		st := &settings[i]
		for _, e := range st.envs {
			settingsByEnv[e] = st
		}
		for _, f := range st.flags {
			settingsByFlag[f] = st
		}
	}
}

func settingForEnv(name string) *setting { return settingsByEnv[name] }

type source struct {
	origin string // "--flag" or "ENV_NAME"
	flag   bool
	value  string
}

// effectiveConfig holds the directories the attach check inspects, resolved
// the way kong's path type does (relative to the process working directory).
type effectiveConfig struct {
	ModelsPath         string
	BackendsPath       string
	BackendsSystemPath string
	DataPath           string
	ConfigDir          string
}

// evaluateConfiguration applies every setting policy to the flags and
// environment of the observed process.
func evaluateConfiguration(v processView, want attachWant) (*effectiveConfig, []*RefusalError) {
	var out []*RefusalError
	sources := map[string][]source{}

	if len(v.Args) == 0 {
		return nil, []*RefusalError{refuse(ConditionProcessObservable, "empty command line")}
	}
	args := v.Args[1:]
	commandSeen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			out = append(out, refuse(ConditionKnownConfiguration, "argument terminator -- makes later arguments model URLs"))
		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(arg[2:], "=")
			st := settingsByFlag[name]
			if st == nil {
				out = append(out, refuse(ConditionKnownConfiguration, "flag --%s is not one OA evaluates", name))
				continue
			}
			if !hasValue {
				if st.boolean {
					value = "true"
					if strings.HasPrefix(name, "no-") {
						value = "false"
					}
				} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					i++
					value = args[i]
				} else {
					out = append(out, refuse(ConditionKnownConfiguration, "flag --%s has no value", name))
					continue
				}
			}
			sources[st.field] = append(sources[st.field], source{origin: "--" + name, flag: true, value: value})
		case strings.HasPrefix(arg, "-") && arg != "-":
			out = append(out, refuse(ConditionKnownConfiguration, "short flag %s is not one OA evaluates", arg))
		case arg == "run" && !commandSeen:
			commandSeen = true
		default:
			commandSeen = true
			// The argument itself is not echoed: a URL can carry credentials.
			out = append(out, refuse(ConditionLocalModels, "a positional argument is present; LocalAI reads it as a model URL or another command"))
		}
	}
	for _, kv := range v.Env {
		name, value, _ := strings.Cut(kv, "=")
		if st := settingsByEnv[name]; st != nil {
			sources[st.field] = append(sources[st.field], source{origin: name, value: value})
		}
	}

	home := envValue(v.Env, "HOME")
	cfg := &effectiveConfig{}
	pathDefaults := map[string]*string{}
	resolved := map[string]string{
		"ModelsPath":         path.Join(v.Cwd, "models"),
		"BackendsPath":       path.Join(v.Cwd, "backends"),
		"BackendsSystemPath": "/var/lib/local-ai/backends",
		"DataPath":           path.Join(v.Cwd, "data"),
		"LocalaiConfigDir":   path.Join(v.Cwd, "configuration"),
	}
	pathDefaults["ModelsPath"] = &cfg.ModelsPath
	pathDefaults["BackendsPath"] = &cfg.BackendsPath
	pathDefaults["BackendsSystemPath"] = &cfg.BackendsSystemPath
	pathDefaults["DataPath"] = &cfg.DataPath
	pathDefaults["LocalaiConfigDir"] = &cfg.ConfigDir
	ambiguous := false

	for i := range settings {
		st := &settings[i]
		srcs := sources[st.field]
		switch st.pol {
		case polNeutral:
		case polPath:
			var first string
			for j, src := range srcs {
				p := expandPath(src.value, v.Cwd, home)
				if j == 0 {
					first = p
				} else if p != first {
					out = append(out, refuse(ConditionKnownConfiguration, "%s and %s name different paths for %s", srcs[0].origin, src.origin, st.field))
					ambiguous = true
				}
			}
			if len(srcs) > 0 {
				resolved[st.field] = first
			}
		case polAddress:
			if len(srcs) == 0 {
				out = append(out, refuse(st.cond, "no address is set; LocalAI binds :8080 on every interface"))
			}
			for _, src := range srcs {
				if !loopbackAddress(src.value, want.Port) {
					out = append(out, refuse(st.cond, "%s=%q is not 127.0.0.1:%s", src.origin, src.value, want.Port))
				}
			}
		case polAPIKey:
			envSet := false
			for _, src := range srcs {
				if src.flag {
					out = append(out, refuse(st.cond, "%s puts the API key on the command line, readable by every local user", src.origin))
					continue
				}
				envSet = true
				for _, key := range strings.Split(src.value, ",") {
					if !equalSecret(key, want.APIKey) {
						out = append(out, refuse(st.cond, "%s holds a key other than OA's per-launch key", src.origin))
						break
					}
				}
			}
			if !envSet {
				out = append(out, refuse(st.cond, "LOCALAI_API_KEY is not set; LocalAI accepts unauthenticated requests"))
			}
		case polEmptyList:
			if len(srcs) == 0 {
				out = append(out, refuse(st.cond, "%s is unset; LocalAI v4.10.0 defaults to GitHub-hosted galleries", st.field))
			}
			for _, src := range srcs {
				if strings.TrimSpace(src.value) != "[]" {
					out = append(out, refuse(st.cond, "%s is not the empty list []", src.origin))
				}
			}
		case polMustTrue, polMustFalse, polFalseIfSet:
			required := st.pol == polMustTrue
			if len(srcs) == 0 && st.pol != polFalseIfSet {
				out = append(out, refuse(st.cond, "%s is unset; LocalAI v4.10.0 defaults it to %t", st.field, !required))
			}
			for _, src := range srcs {
				b, err := strconv.ParseBool(src.value)
				if err != nil || b != required {
					out = append(out, refuse(st.cond, "%s=%q; required %t", src.origin, src.value, required))
				}
			}
		case polAbsent:
			for _, src := range srcs {
				out = append(out, refuse(st.cond, "%s is set", src.origin))
			}
		}
	}
	if ambiguous {
		return nil, out
	}
	for field, dst := range pathDefaults {
		*dst = resolved[field]
	}
	return cfg, out
}

// expandPath mirrors kong.ExpandPath: a leading ~ is the home directory and
// a relative path is joined to the working directory.
func expandPath(p, cwd, home string) string {
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = path.Join(home, p[2:])
	}
	if !path.IsAbs(p) {
		p = path.Join(cwd, p)
	}
	return path.Clean(p)
}
