package localai

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// LocalAI model configurations are YAML (core/config/model_config.go). OA
// ships no YAML library, so it reads a block-mapping subset: `key: scalar`,
// `key:` opening a nested mapping or list, and `- scalar` list items. Flow
// collections, anchors, aliases, tags, block scalars, multiple documents,
// duplicate keys and list items holding mappings are refused. A file LocalAI
// would read differently from this subset is refused, never guessed.

type localModel struct {
	Name string
	File string
}

// modelBackend is the only backend the review approved.
const modelBackend = "cpu-llama-cpp"

// modelValueKind constrains the value at an admitted path.
type modelValueKind int

const (
	valueContainer modelValueKind = iota // a `key:` block; no scalar
	valueText                            // any local scalar
	valueWord                            // letters, digits and underscores
	valueInt
	valueBool
)

// admittedModelPaths are the paths of the reviewed HP6 model configuration
// (.build/headless-providers/localai/hp6-20260930/qwen3-0.6b-q4_0.yaml).
// Every other path refuses: LocalAI's ModelConfig also carries env, proxy,
// mcp.remote, mcp.stdio, download_files, trust_remote_code and backend
// options, each of which changes what runs or where work goes.
var admittedModelPaths = map[string]modelValueKind{
	"name":                            valueText,
	"backend":                         valueText,
	"known_usecases":                  valueContainer,
	"known_usecases[]":                valueWord,
	"parameters":                      valueContainer,
	"parameters.model":                valueText,
	"context_size":                    valueInt,
	"mmap":                            valueBool,
	"stopwords":                       valueContainer,
	"stopwords[]":                     valueText,
	"options":                         valueContainer,
	"options[]":                       valueText, // checked by checkModelOption
	"function":                        valueContainer,
	"function.grammar":                valueContainer,
	"function.grammar.disable":        valueBool,
	"template":                        valueContainer,
	"template.use_tokenizer_template": valueBool,
}

var (
	yamlKey        = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	yamlWord       = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	localModelFile = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.gguf$`)
)

// remoteValue reports scalars that LocalAI's downloader treats as remote
// sources (pkg/downloader/uri.go prefixes) or that name a URL.
func remoteValue(v string) bool {
	lower := strings.ToLower(v)
	if strings.Contains(lower, "://") {
		return true
	}
	for _, prefix := range []string{"github:", "hf.co/", "huggingface.co/", "ollama:", "oci:", "huggingface:", "hf:"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

type yamlFrame struct {
	indent int
	path   string
}

// scanModelYAML flattens the supported subset into path → values. A list
// item appends to "<path>[]".
func scanModelYAML(data []byte) (map[string][]string, string) {
	out := map[string][]string{}
	var stack []yamlFrame
	open := "" // path of the last `key:` without a scalar
	openIndent := -1
	for n, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.ContainsRune(line[:len(line)-len(trimmed)], '\t') || strings.HasPrefix(trimmed, "\t") {
			return nil, lineErr(n, "tab indentation")
		}
		indent := len(line) - len(trimmed)
		if trimmed == "---" || trimmed == "..." || strings.HasPrefix(trimmed, "%") {
			return nil, lineErr(n, "document markers and directives are unsupported")
		}
		for len(stack) > 0 && indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		parent := ""
		if len(stack) > 0 {
			parent = stack[len(stack)-1].path
		}
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			if open == "" || indent < openIndent {
				return nil, lineErr(n, "list item without an owning key")
			}
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if k, _, isMap := cutKey(item); isMap && yamlKey.MatchString(k) {
				return nil, lineErr(n, "list items holding mappings are unsupported")
			}
			value, bad := yamlScalar(item)
			if bad != "" {
				return nil, lineErr(n, bad)
			}
			out[open+"[]"] = append(out[open+"[]"], value)
			continue
		}
		key, rest, ok := cutKey(trimmed)
		if !ok || !yamlKey.MatchString(key) || key == "<<" {
			return nil, lineErr(n, "unsupported line")
		}
		full := key
		if parent != "" {
			full = parent + "." + key
		}
		if _, dup := out[full]; dup {
			return nil, lineErr(n, "duplicate key "+full)
		}
		if rest == "" {
			out[full] = nil
			stack = append(stack, yamlFrame{indent: indent, path: full})
			open, openIndent = full, indent
			continue
		}
		value, bad := yamlScalar(rest)
		if bad != "" {
			return nil, lineErr(n, bad)
		}
		out[full] = []string{value}
		open, openIndent = "", -1
	}
	return out, ""
}

// cutKey splits `key: rest` or `key:`.
func cutKey(s string) (string, string, bool) {
	if k, ok := strings.CutSuffix(s, ":"); ok && !strings.Contains(k, ": ") {
		return k, "", true
	}
	k, rest, ok := strings.Cut(s, ": ")
	if !ok {
		return "", "", false
	}
	return k, strings.TrimSpace(rest), true
}

func yamlScalar(s string) (string, string) {
	if s == "" {
		return "", ""
	}
	switch s[0] {
	case '{', '[', '&', '*', '!', '|', '>', '%', '@', '`', '?':
		return "", "unsupported YAML value " + s
	case '"':
		if len(s) < 2 || !strings.HasSuffix(s, `"`) || strings.ContainsAny(s[1:len(s)-1], `"\`) {
			return "", "unsupported double-quoted value"
		}
		return s[1 : len(s)-1], ""
	case '\'':
		if len(s) < 2 || !strings.HasSuffix(s, `'`) || strings.Contains(s[1:len(s)-1], `'`) {
			return "", "unsupported single-quoted value"
		}
		return s[1 : len(s)-1], ""
	}
	if i := strings.Index(s, " #"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s, ""
}

func lineErr(n int, msg string) string {
	return "line " + strconv.Itoa(n+1) + ": " + msg
}

// boolWords are the values backend/cpp/llama-cpp/grpc-server.cpp accepts for
// a boolean option.
var boolWords = map[string]bool{
	"true": true, "1": true, "yes": true, "on": true, "enabled": true,
	"false": true, "0": true, "no": true, "off": true, "disabled": true,
}

// checkModelOption admits one `options:` entry. grpc-server.cpp splits an
// entry with strtok(":") into a name and a value (lines 634-640). Named
// options it knows configure llama.cpp; grpc_servers and rpc_servers add
// remote RPC devices (673-676, 1255-1257); an entry starting with '-' is
// handed verbatim to common_params_parse as a llama-server flag
// (1204-1233), which includes --rpc, --hf-repo and --lora. The reviewed HP6
// configuration needs use_jinja only.
func checkModelOption(shown, entry string) *RefusalError {
	name, value, _ := strings.Cut(strings.TrimLeft(entry, ":"), ":")
	if i := strings.IndexByte(value, ':'); i >= 0 {
		value = value[:i]
	}
	switch {
	case strings.HasPrefix(name, "-"):
		if strings.TrimLeft(name, "-") == "rpc" {
			return refuse(ConditionNoRemoteBackends, "%s: option %s sends work to remote llama.cpp RPC servers", shown, name)
		}
		return refuse(ConditionLocalModels, "%s: option %s passes a raw llama.cpp flag", shown, name)
	case name == "grpc_servers" || name == "rpc_servers":
		return refuse(ConditionNoRemoteBackends, "%s: option %s sends work to remote llama.cpp RPC servers", shown, name)
	case name == "use_jinja" || name == "jinja":
		if value == "" && !strings.Contains(entry, ":") || boolWords[value] {
			return nil
		}
		return refuse(ConditionLocalModels, "%s: option %s has an unreviewed value", shown, name)
	}
	return refuse(ConditionLocalModels, "%s: option %s is not in the reviewed configuration", shown, name)
}

// checkModelConfig accepts a model configuration that names a local GGUF
// file on the reviewed backend and nothing remote.
func checkModelConfig(shown string, data []byte) (localModel, *RefusalError) {
	fields, bad := scanModelYAML(data)
	if bad != "" {
		return localModel{}, refuse(ConditionLocalModels, "%s: %s", shown, bad)
	}
	for _, entry := range fields["options[]"] {
		if r := checkModelOption(shown, entry); r != nil {
			return localModel{}, r
		}
	}
	// env: is appended to the backend process's environment
	// (core/config/model_config.go:175-176, core/backend/options.go:247-248
	// and 534-540, pkg/model/process.go:292-296). A named variable that the
	// process environment check forbids keeps its condition; any other
	// variable is refused as an unreviewed environment change.
	if _, ok := fields["env"]; ok {
		for key := range fields {
			if name, isVar := strings.CutPrefix(key, "env."); isVar {
				if c, bad := forbiddenEnv(name); bad {
					return localModel{}, refuse(c, "%s: env sets %s for the backend", shown, name)
				}
			}
		}
		return localModel{}, refuse(ConditionCleanEnvironment, "%s: env changes the backend's environment", shown)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		kind, admitted := admittedModelPaths[key]
		if !admitted {
			return localModel{}, refuse(ConditionLocalModels, "%s: key %s is not in the reviewed model configuration", shown, key)
		}
		values := fields[key]
		if kind == valueContainer && len(values) > 0 {
			return localModel{}, refuse(ConditionLocalModels, "%s: %s holds a scalar", shown, key)
		}
		for _, v := range values {
			if remoteValue(v) {
				return localModel{}, refuse(ConditionLocalModels, "%s: %s names a remote source", shown, key)
			}
			ok := true
			switch kind {
			case valueWord:
				ok = yamlWord.MatchString(v)
			case valueInt:
				_, err := strconv.Atoi(v)
				ok = err == nil
			case valueBool:
				ok = v == "true" || v == "false"
			}
			if !ok {
				return localModel{}, refuse(ConditionLocalModels, "%s: %s has an unreviewed value", shown, key)
			}
		}
	}
	one := func(key string) string {
		if v := fields[key]; len(v) == 1 {
			return v[0]
		}
		return ""
	}
	m := localModel{Name: one("name"), File: one("parameters.model")}
	if m.Name == "" {
		return localModel{}, refuse(ConditionLocalModels, "%s: no model name", shown)
	}
	if backend := one("backend"); backend != modelBackend {
		return localModel{}, refuse(ConditionLocalModels, "%s: backend %q is not %s", shown, backend, modelBackend)
	}
	if !localModelFile.MatchString(m.File) {
		return localModel{}, refuse(ConditionLocalModels, "%s: parameters.model %q is not a GGUF file name in the models directory", shown, m.File)
	}
	return m, nil
}
