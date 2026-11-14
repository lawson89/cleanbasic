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
	if e.usedStdRegex {
		sb.WriteString(stdRegexHelpers)
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

func cbStdFsIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func cbStdFsIsSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && (info.Mode()&os.ModeSymlink) != 0
}

func cbStdFsJoin(a string, b string) string {
	return filepath.Join(a, b)
}

func cbStdFsBase(path string) string {
	return filepath.Base(path)
}

func cbStdFsDir(path string) string {
	return filepath.Dir(path)
}

func cbStdFsExt(path string) string {
	return filepath.Ext(path)
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

func cbStdOsReadStdin() (string, error) {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", CbError{msg: err.Error()}
	}
	return string(b), nil
}

func cbStdOsCwd() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", CbError{msg: err.Error()}
	}
	return dir, nil
}

`

const stdRegexHelpers = `type CbStdRegex struct {
	re *regexp.Regexp
}

func cbStdRegexCompile(pattern string) (CbStdRegex, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return CbStdRegex{}, CbError{msg: err.Error()}
	}
	return CbStdRegex{re: re}, nil
}

func cbStdRegexMatch(pattern string, text string) (bool, error) {
	matched, err := regexp.MatchString(pattern, text)
	if err != nil {
		return false, CbError{msg: err.Error()}
	}
	return matched, nil
}

func (r CbStdRegex) isMatch(text string) bool {
	if r.re == nil {
		return false
	}
	return r.re.MatchString(text)
}

func (r CbStdRegex) find(text string) string {
	if r.re == nil {
		return ""
	}
	return r.re.FindString(text)
}

func (r CbStdRegex) findAll(text string) []string {
	if r.re == nil {
		return []string{}
	}
	res := r.re.FindAllString(text, -1)
	if res == nil {
		return []string{}
	}
	return res
}

func (r CbStdRegex) replaceAll(text string, repl string) string {
	if r.re == nil {
		return text
	}
	return r.re.ReplaceAllString(text, repl)
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

func (j CbStdJsonValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.raw)
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

type cbStdCliParserState struct {
	name        string
	version     string
	description string
	flags       []cbCliFlagDef
	options     []cbCliOptDef
	args        []cbCliArgDef
}

type CbStdCliParser struct {
	state *cbStdCliParserState
}

type CbStdCliArgs struct {
	helpRequested bool
	positionals   []string
	flags         map[string]bool
	options       map[string]string
	namedArgs     map[string]string
}

func cbStdCliNewParser(name, version, description string) CbStdCliParser {
	return CbStdCliParser{
		state: &cbStdCliParserState{
			name:        name,
			version:     version,
			description: description,
		},
	}
}

func (p *CbStdCliParser) ensureState() *cbStdCliParserState {
	if p.state == nil {
		p.state = &cbStdCliParserState{}
	}
	return p.state
}

func (p *CbStdCliParser) addFlag(longName, short, desc string) {
	s := p.ensureState()
	s.flags = append(s.flags, cbCliFlagDef{longName: longName, short: short, desc: desc})
}

func (p *CbStdCliParser) addOption(longName, short, desc, defaultVal string) {
	s := p.ensureState()
	s.options = append(s.options, cbCliOptDef{longName: longName, short: short, desc: desc, defaultVal: defaultVal})
}

func (p *CbStdCliParser) addArg(name, desc string) {
	s := p.ensureState()
	s.args = append(s.args, cbCliArgDef{name: name, desc: desc})
}

func (p CbStdCliParser) printUsage() {
	s := p.state
	if s == nil {
		s = &cbStdCliParserState{}
	}
	fmt.Printf("%s %s - %s\n", s.name, s.version, s.description)
	fmt.Printf("Usage: %s [options]", s.name)
	for _, a := range s.args {
		fmt.Printf(" <%s>", a.name)
	}
	fmt.Println()
	for _, f := range s.flags {
		fmt.Printf("  -%s, --%s\t%s\n", f.short, f.longName, f.desc)
	}
	for _, o := range s.options {
		fmt.Printf("  -%s, --%s\t%s (default: %s)\n", o.short, o.longName, o.desc, o.defaultVal)
	}
	for _, a := range s.args {
		fmt.Printf("  %s\t%s\n", a.name, a.desc)
	}
}

func (p CbStdCliParser) parse() (CbStdCliArgs, error) {
	var raw []string
	if len(os.Args) > 1 {
		raw = os.Args[1:]
	}
	return p.parseArgs(raw)
}

func (p CbStdCliParser) parseArgs(raw []string) (CbStdCliArgs, error) {
	s := p.state
	if s == nil {
		s = &cbStdCliParserState{}
	}
	res := CbStdCliArgs{
		flags:     make(map[string]bool),
		options:   make(map[string]string),
		namedArgs: make(map[string]string),
	}
	for _, o := range s.options {
		res.options[o.longName] = o.defaultVal
	}

	flagByLong := make(map[string]string)
	flagByShort := make(map[string]string)
	for _, f := range s.flags {
		flagByLong[f.longName] = f.longName
		if f.short != "" {
			flagByShort[f.short] = f.longName
		}
	}

	optByLong := make(map[string]string)
	optByShort := make(map[string]string)
	for _, o := range s.options {
		optByLong[o.longName] = o.longName
		if o.short != "" {
			optByShort[o.short] = o.longName
		}
	}

	stopFlags := false
	for i := 0; i < len(raw); i++ {
		tok := raw[i]
		if stopFlags {
			res.positionals = append(res.positionals, tok)
			continue
		}
		if tok == "--" {
			stopFlags = true
			continue
		}
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
			if len(short) > 1 {
				allFlags := true
				for _, ch := range short {
					sCh := string(ch)
					if sCh == "h" {
						continue
					}
					if _, ok := flagByShort[sCh]; !ok {
						allFlags = false
						break
					}
				}
				if allFlags {
					for _, ch := range short {
						sCh := string(ch)
						if sCh == "h" {
							res.helpRequested = true
							return res, nil
						}
						res.flags[flagByShort[sCh]] = true
					}
					continue
				}
			}
			return res, CbError{msg: fmt.Sprintf("unknown flag -%s", short)}
		}
		res.positionals = append(res.positionals, tok)
	}

	for idx, a := range s.args {
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
