package localai

// reviewPins are the artifacts one security review approved. A new LocalAI
// version, backend digest or backend set needs a new review and a new pin set
// (research/security-reviews/localai-4.10.0.md, "Re-review on any version,
// backend digest or backend set change").
type reviewPins struct {
	ProgramSHA256 string
	BackendName   string
	BackendDigest string
	BackendURI    string
	// BackendFiles maps each path under the backend directory to its
	// lowercase SHA-256, "link:<target>" for a symlink or "dir". metadata.json
	// is compared by field because LocalAI writes its install time into it.
	BackendFiles map[string]string
}

// reviewedV4100 is LocalAI v4.10.0 (tag commit
// 7ad0cbf259f0c7bf9920fe2438fc3630ecd6c672, asset local-ai-v4.10.0-linux-amd64)
// with the cpu-llama-cpp backend from
// quay.io/go-skynet/local-ai-backends@sha256:9d6b85ce…cfda74, reviewed
// 2026-10-01. The file hashes were read from the reviewed installation
// /root/.cache/openabstractions/hp6-localai-20260930/backends/cpu-llama-cpp
// in WSL on 2026-10-01; run.sh matches the review's
// backend/cpp/llama-cpp/run.sh digest at the tag.
var reviewedV4100 = reviewPins{
	ProgramSHA256: "412cde3dc89b0bf3c643e9c07c40a32cad0d17a1008f818cd4243bd03be8c006",
	BackendName:   "cpu-llama-cpp",
	BackendDigest: "sha256:9d6b85ce9a0463e715a9cff7cbf4e8327182e44203e9f926988972e340cfda74",
	BackendURI:    "quay.io/go-skynet/local-ai-backends@sha256:9d6b85ce9a0463e715a9cff7cbf4e8327182e44203e9f926988972e340cfda74",
	BackendFiles: map[string]string{
		"lib":                               "dir",
		"lib/ld.so":                         "c20a2dc8917c755f02b94049356320fe1f62ac7d9f8994731f807d9df39302da",
		"lib/libc.so.6":                     "3a15d66867d83762c7f2f1e37359cb8f6c5743edb369c65285cb0b1c4f7498bf",
		"lib/libdl.so.2":                    "c2994527172cc23b7fb0f7d611b71e00a37da27af12c2e5e9e0dc1a742886c96",
		"lib/libgcc_s.so.1":                 "d93224d2b0dab4247598be683adca02f5cf00586f99c187579cd7e92058fb7cb",
		"lib/libggml-base.so":               "link:libggml-base.so.0",
		"lib/libggml-base.so.0":             "link:libggml-base.so.0.24.0",
		"lib/libggml-base.so.0.24.0":        "11a9c971dfcde2fbc6418579ba28f4d89471109c964fafa3ee9023684e624ac6",
		"lib/libggml-cpu-alderlake.so":      "afb783e3cd288bcfdca7e962d6063b32aca62e7d8c46504956a5f5add3a1e978",
		"lib/libggml-cpu-cannonlake.so":     "837f9b17ce104dd78b46c2ffc285de3bc881dcc0ce64c4cdef1aa90688edaf77",
		"lib/libggml-cpu-cascadelake.so":    "f7747be012ecabe37e132924922fd522501e485ed922f4e5e708f4f003c623ee",
		"lib/libggml-cpu-cooperlake.so":     "b32013fea0e8ca4b7f9bc162fbaf731de08f4fb4b8d530c6194db14f71aa186a",
		"lib/libggml-cpu-haswell.so":        "4a1f2194cefef521b349df92a060ea46358e05b3695b1cf964918a82dd9b9233",
		"lib/libggml-cpu-icelake.so":        "4a9fecc6b084a2d74ef478f63d16023d33f8009c9463baf180f12d13c9bfc4c5",
		"lib/libggml-cpu-ivybridge.so":      "9f113a1954e9aebad93dc25ef1f67c23a751fb9b9500867871465e692398e951",
		"lib/libggml-cpu-piledriver.so":     "012081fac1055c13907722c45359ac6db10d489da686768811b2405c452504d5",
		"lib/libggml-cpu-sandybridge.so":    "f45de99703a14bb4247265128102355097c2ee3729cf6d9e539ee07c58a77303",
		"lib/libggml-cpu-sapphirerapids.so": "758fcb1aa24ac12259d5165bf26f0162911bf6c976b59a41596b6dff61aee2fc",
		"lib/libggml-cpu-skylakex.so":       "0b6af0a4d4848bc65153816beac36c74ccadeefb04110f1c35b9c05ff6110d58",
		"lib/libggml-cpu-sse42.so":          "363eed18f8351d86a8dd7faa50847e45fe640d680bdb2b91280a47cea457c29a",
		"lib/libggml-cpu-x64.so":            "c69a6eb3ab01fba331c2286cc628299ee86af4baf552e9bb7c73365508788d12",
		"lib/libggml-cpu-zen4.so":           "b135682b6427421a756fb76e0c45f2f91fc1b6514e7de0a026080bb47c59e529",
		"lib/libggml.so":                    "link:libggml.so.0",
		"lib/libggml.so.0":                  "link:libggml.so.0.24.0",
		"lib/libggml.so.0.24.0":             "b86ecd11bb453653c118da1060a858822cc811792c4a1a8031d642872ac53bd8",
		"lib/libgomp.so.1":                  "135f3c8f006d2fe5e68e51281c7974cb991a03de3bfb3593d68d174dfcf854d1",
		"lib/libllama-common.so":            "link:libllama-common.so.0",
		"lib/libllama-common.so.0":          "link:libllama-common.so.0.4.1",
		"lib/libllama-common.so.0.4.1":      "780b3635081efb32c6307570efa66bf67ff0a6144599be33bbbb3f6770b128ac",
		"lib/libllama.so":                   "link:libllama.so.0",
		"lib/libllama.so.0":                 "link:libllama.so.0.4.1",
		"lib/libllama.so.0.4.1":             "75eb7af99a26d1996102953178840a1a3ef8ff3ff8cf3fefa98141143eb7bb84",
		"lib/libm.so.6":                     "fce00b6f25f459cf4ae0b7fae4257909a1a8a86f9149e7c029a5d617baf1ccd0",
		"lib/libmtmd.so":                    "link:libmtmd.so.0",
		"lib/libmtmd.so.0":                  "link:libmtmd.so.0.4.1",
		"lib/libmtmd.so.0.4.1":              "84ac60f20410ac1a1bce50392c94aa678a861cc15e3a1af9a9085d3aa6480bd0",
		"lib/libpthread.so.0":               "90491a21ce44efcf3bd4b65ec82501c091d7ebe7f8d9b341dd01d721001d9ecd",
		"lib/librt.so.1":                    "6d072187d88e092e81561a44c03a4a41397319022c6d43519757cc051c576d82",
		"lib/libstdc++.so.6":                "1fd75fe70354a416d75aef22bcae68c47bd25d20e2d0568c30b1a9838cf62f11",
		"llama-cpp-cpu-all":                 "9bce74e8ab74e4659318d8ad62b17b286ce81c32d3ae538970ea8a6206c725bc",
		"llama-cpp-grpc":                    "cd2f325f8101d92fb92a9f500a8d18e28a9dd1913585548c66c64bda6e55ba71",
		"llama-cpp-rpc-server":              "4accb72ba4de0baa7d26dbc5bc1847828c4c6e89ade800f0361c4124712411bd",
		"run.sh":                            "d74a3bb9b53bd833dc11ea4f820d324977d24d51135c43a34fc67ce549dad568",
	},
}
