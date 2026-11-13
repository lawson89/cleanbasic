package codegen

import "strings"

func (e *emitter) emitStdlibHelpers(sb *strings.Builder) {
	if e.usedStdFs {
		sb.WriteString(stdFsHelpers)
	}
	if e.usedStdOs {
		sb.WriteString(stdOsHelpers)
	}
	if e.usedStdOsExec {
		sb.WriteString(stdOsExecHelpers)
	}
	if e.usedStdJson {
		sb.WriteString(stdJsonHelpers)
	}
	if e.usedStdCli {
		sb.WriteString(stdCliHelpers)
	}
}

const stdFsHelpers = `func cbStdFsReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", CbError{msg: err.Error()}
	}
	return string(b), nil
}

func cbStdFsWriteFile(path string, content string) error {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return CbError{msg: err.Error()}
	}
	return nil
}

func cbStdFsExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cbStdFsMkdirAll(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return CbError{msg: err.Error()}
	}
	return nil
}

func cbStdFsListDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, CbError{msg: err.Error()}
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out, nil
}

`

const stdOsHelpers = `type CbStdOsExecResult struct {
	exitCode int64
	stdout   string
	stderr   string
}

func cbStdOsArgs() []string {
	if len(os.Args) <= 1 {
		return []string{}
	}
	return append([]string{}, os.Args[1:]...)
}

func cbStdOsExit(code int64) {
	os.Exit(int(code))
}

func cbStdOsGetEnv(key string) string {
	return os.Getenv(key)
}

`

const stdOsExecHelpers = `func cbStdOsExec(cmd string, args []string) (CbStdOsExecResult, error) {
	c := exec.Command(cmd, args...)
	var outBuf, errBuf bytes.Buffer
	c.Stdout = &outBuf
	c.Stderr = &errBuf
	err := c.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return CbStdOsExecResult{
				exitCode: int64(exitErr.ExitCode()),
				stdout:   outBuf.String(),
				stderr:   errBuf.String(),
			}, nil
		}
		return CbStdOsExecResult{}, CbError{msg: err.Error()}
	}
	return CbStdOsExecResult{
		exitCode: 0,
		stdout:   outBuf.String(),
		stderr:   errBuf.String(),
	}, nil
}

`

const stdJsonHelpers = `type CbStdJsonValue struct {
	raw any
}

func cbStdJsonParse(raw string) (CbStdJsonValue, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return CbStdJsonValue{}, CbError{msg: err.Error()}
	}
	return CbStdJsonValue{raw: v}, nil
}

func cbStdJsonStringify(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func (j CbStdJsonValue) isNull() bool {
	return j.raw == nil
}

func (j CbStdJsonValue) asStr() string {
	if s, ok := j.raw.(string); ok {
		return s
	}
	if j.raw == nil {
		return ""
	}
	return fmt.Sprint(j.raw)
}

func (j CbStdJsonValue) asInt() int64 {
	switch v := j.raw.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	default:
		return 0
	}
}

func (j CbStdJsonValue) asFloat() float64 {
	if f, ok := j.raw.(float64); ok {
		return f
	}
	return 0
}

func (j CbStdJsonValue) asBool() bool {
	if b, ok := j.raw.(bool); ok {
		return b
	}
	return false
}

func (j CbStdJsonValue) has(key string) bool {
	m, ok := j.raw.(map[string]any)
	if !ok {
		return false
	}
	_, exists := m[key]
	return exists
}

func (j CbStdJsonValue) get(key string) CbStdJsonValue {
	m, ok := j.raw.(map[string]any)
	if !ok {
		return CbStdJsonValue{}
	}
	return CbStdJsonValue{raw: m[key]}
}

func (j CbStdJsonValue) asList() []CbStdJsonValue {
	arr, ok := j.raw.([]any)
	if !ok {
		return []CbStdJsonValue{}
	}
	out := make([]CbStdJsonValue, len(arr))
	for i, el := range arr {
		out[i] = CbStdJsonValue{raw: el}
	}
	return out
}

func (j CbStdJsonValue) asDict() map[string]CbStdJsonValue {
	m, ok := j.raw.(map[string]any)
	if !ok {
		return map[string]CbStdJsonValue{}
	}
	out := make(map[string]CbStdJsonValue, len(m))
	for k, v := range m {
		out[k] = CbStdJsonValue{raw: v}
	}
	return out
}

`

const stdCliHelpers = `type cbCliFlagDef struct {
	longName string
	short    string
	desc     string
}

type cbCliOptDef struct {
	longName   string
	short      string
	desc       string
	defaultVal string
}

type cbCliArgDef struct {
	name string
	desc string
}

type CbStdCliParser struct {
	name        string
	version     string
	description string
	flags       []cbCliFlagDef
	options     []cbCliOptDef
	args        []cbCliArgDef
}

type CbStdCliArgs struct {
	helpRequested bool
	positionals   []string
	flags         map[string]bool
	options       map[string]string
	namedArgs     map[string]string
}

func cbStdCliNewParser(name, version, description string) *CbStdCliParser {
	return &CbStdCliParser{
		name:        name,
		version:     version,
		description: description,
	}
}

func (p *CbStdCliParser) addFlag(longName, short, desc string) {
	p.flags = append(p.flags, cbCliFlagDef{longName: longName, short: short, desc: desc})
}

func (p *CbStdCliParser) addOption(longName, short, desc, defaultVal string) {
	p.options = append(p.options, cbCliOptDef{longName: longName, short: short, desc: desc, defaultVal: defaultVal})
}

func (p *CbStdCliParser) addArg(name, desc string) {
	p.args = append(p.args, cbCliArgDef{name: name, desc: desc})
}

func (p *CbStdCliParser) printUsage() {
	fmt.Printf("%s %s - %s\n", p.name, p.version, p.description)
	fmt.Printf("Usage: %s [options]", p.name)
	for _, a := range p.args {
		fmt.Printf(" <%s>", a.name)
	}
	fmt.Println()
	for _, f := range p.flags {
		fmt.Printf("  -%s, --%s\t%s\n", f.short, f.longName, f.desc)
	}
	for _, o := range p.options {
		fmt.Printf("  -%s, --%s\t%s (default: %s)\n", o.short, o.longName, o.desc, o.defaultVal)
	}
	for _, a := range p.args {
		fmt.Printf("  %s\t%s\n", a.name, a.desc)
	}
}

func (p *CbStdCliParser) parse() (CbStdCliArgs, error) {
	var raw []string
	if len(os.Args) > 1 {
		raw = os.Args[1:]
	}
	return p.parseArgs(raw)
}

func (p *CbStdCliParser) parseArgs(raw []string) (CbStdCliArgs, error) {
	res := CbStdCliArgs{
		flags:     make(map[string]bool),
		options:   make(map[string]string),
		namedArgs: make(map[string]string),
	}
	for _, o := range p.options {
		res.options[o.longName] = o.defaultVal
	}

	flagByLong := make(map[string]string)
	flagByShort := make(map[string]string)
	for _, f := range p.flags {
		flagByLong[f.longName] = f.longName
		if f.short != "" {
			flagByShort[f.short] = f.longName
		}
	}

	optByLong := make(map[string]string)
	optByShort := make(map[string]string)
	for _, o := range p.options {
		optByLong[o.longName] = o.longName
		if o.short != "" {
			optByShort[o.short] = o.longName
		}
	}

	for i := 0; i < len(raw); i++ {
		tok := raw[i]
		if tok == "--help" || tok == "-h" {
			res.helpRequested = true
			return res, nil
		}
		if strings.HasPrefix(tok, "--") {
			body := tok[2:]
			if eq := strings.IndexByte(body, '='); eq >= 0 {
				k := body[:eq]
				v := body[eq+1:]
				if longName, ok := optByLong[k]; ok {
					res.options[longName] = v
					continue
				}
				return res, CbError{msg: fmt.Sprintf("unknown option --%s", k)}
			}
			if longName, ok := flagByLong[body]; ok {
				res.flags[longName] = true
				continue
			}
			if longName, ok := optByLong[body]; ok {
				if i+1 >= len(raw) {
					return res, CbError{msg: fmt.Sprintf("option --%s requires a value", body)}
				}
				i++
				res.options[longName] = raw[i]
				continue
			}
			return res, CbError{msg: fmt.Sprintf("unknown flag --%s", body)}
		}
		if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			short := tok[1:]
			if longName, ok := flagByShort[short]; ok {
				res.flags[longName] = true
				continue
			}
			if longName, ok := optByShort[short]; ok {
				if i+1 >= len(raw) {
					return res, CbError{msg: fmt.Sprintf("option -%s requires a value", short)}
				}
				i++
				res.options[longName] = raw[i]
				continue
			}
			return res, CbError{msg: fmt.Sprintf("unknown flag -%s", short)}
		}
		res.positionals = append(res.positionals, tok)
	}

	for idx, a := range p.args {
		if idx >= len(res.positionals) {
			return res, CbError{msg: fmt.Sprintf("missing required argument %q", a.name)}
		}
		res.namedArgs[a.name] = res.positionals[idx]
	}
	return res, nil
}

func (a CbStdCliArgs) hasFlag(name string) bool {
	return a.flags[name]
}

func (a CbStdCliArgs) getOption(name string) string {
	return a.options[name]
}

func (a CbStdCliArgs) getArg(name string) string {
	return a.namedArgs[name]
}

`
