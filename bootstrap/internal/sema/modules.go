package sema

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"cleanbasic/bootstrap/internal/ast"
	"cleanbasic/bootstrap/internal/parser"
	"cleanbasic/bootstrap/internal/token"
)

// ModuleInfo holds the AST, import aliases, and symbol tables for a single CleanBasic or stdlib module.
type ModuleInfo struct {
	ID          string
	FilePath    string
	SanitizedID string
	IsEntry     bool
	IsStdlib    bool
	Program     *ast.Program
	Imports     map[string]*ModuleInfo
	Functions   map[string]*FnSymbol
	Structs     map[string]*StructSymbol
	Enums       map[string]*EnumSymbol
	Unions      map[string]*UnionSymbol
	GlobalVars  *Scope
}

func newModuleInfo(id, filePath string, isEntry, isStdlib bool, prog *ast.Program) *ModuleInfo {
	return &ModuleInfo{
		ID:          id,
		FilePath:    filePath,
		SanitizedID: sanitizeModuleID(id),
		IsEntry:     isEntry,
		IsStdlib:    isStdlib,
		Program:     prog,
		Imports:     make(map[string]*ModuleInfo),
		Functions:   make(map[string]*FnSymbol),
		Structs:     make(map[string]*StructSymbol),
		Enums:       make(map[string]*EnumSymbol),
		Unions:      make(map[string]*UnionSymbol),
		GlobalVars:  NewScope(nil),
	}
}

func sanitizeModuleID(id string) string {
	var sb strings.Builder
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('_')
		}
	}
	return sb.String()
}

func (m *ModuleInfo) symbolGoName(name string) string {
	if m.IsEntry {
		return name
	}
	return fmt.Sprintf("cb_%s__%s", m.SanitizedID, name)
}

type moduleLoader struct {
	rootDir   string
	state     map[string]int // 0=unvisited, 1=visiting, 2=done
	modules   map[string]*ModuleInfo
	byID      map[string]*ModuleInfo
	stdMods   map[string]*ModuleInfo
	topoOrder []*ModuleInfo
	stack     []string
}

func newModuleLoader(rootDir string) *moduleLoader {
	return &moduleLoader{
		rootDir: rootDir,
		state:   make(map[string]int),
		modules: make(map[string]*ModuleInfo),
		byID:    make(map[string]*ModuleInfo),
		stdMods: make(map[string]*ModuleInfo),
	}
}

func (l *moduleLoader) canonicalID(absPath string) string {
	if l.rootDir == "" {
		return strings.TrimSuffix(filepath.ToSlash(absPath), ".cb")
	}
	rel, err := filepath.Rel(l.rootDir, absPath)
	if err != nil {
		return strings.TrimSuffix(filepath.ToSlash(absPath), ".cb")
	}
	return strings.TrimSuffix(filepath.ToSlash(rel), ".cb")
}

func (l *moduleLoader) loadEntry(entryPath string, entryProg *ast.Program) ([]*ModuleInfo, *ModuleInfo, map[string]*ModuleInfo, error) {
	absEntry := entryPath
	if entryPath != "" {
		if abs, err := filepath.Abs(entryPath); err == nil {
			absEntry = filepath.Clean(abs)
		}
	}
	mod, err := l.visitFile(absEntry, entryProg, true)
	if err != nil {
		return nil, nil, nil, err
	}
	return l.topoOrder, mod, l.byID, nil
}

func (l *moduleLoader) visitFile(absPath string, preParsed *ast.Program, isEntry bool) (*ModuleInfo, error) {
	key := absPath
	if key == "" {
		key = "<memory>"
	}
	modID := l.canonicalID(key)
	if key == "<memory>" {
		modID = "main"
	}

	if l.state[key] == 2 {
		return l.modules[key], nil
	}

	l.state[key] = 1
	l.stack = append(l.stack, modID)
	defer func() {
		l.stack = l.stack[:len(l.stack)-1]
	}()

	prog := preParsed
	if prog == nil {
		raw, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", absPath, err)
		}
		parsed, err := parser.Parse(string(raw), absPath)
		if err != nil {
			return nil, err
		}
		prog = parsed
	}

	mod := newModuleInfo(modID, absPath, isEntry, false, prog)
	l.modules[key] = mod
	l.byID[modID] = mod

	curDir := "."
	if absPath != "" && absPath != "<memory>" {
		curDir = filepath.Dir(absPath)
	}

	for _, decl := range prog.Decls {
		imp, ok := decl.(*ast.ImportDecl)
		if !ok || imp.IsHeader {
			continue
		}

		alias := imp.Alias
		if alias == "" {
			if strings.HasPrefix(imp.Path, "std/") {
				alias = path.Base(imp.Path)
			} else {
				base := path.Base(filepath.ToSlash(imp.Path))
				alias = strings.TrimSuffix(base, ".cb")
			}
		}

		if existing, dup := mod.Imports[alias]; dup {
			return nil, newSemaError(imp.Pos, "duplicate import alias %q (already bound to %q)", alias, existing.ID)
		}

		if strings.HasPrefix(imp.Path, "std/") {
			stdMod, err := l.getOrCreateStdlib(imp.Path, imp.Pos)
			if err != nil {
				return nil, err
			}
			mod.Imports[alias] = stdMod
			continue
		}

		if absPath == "" || absPath == "<memory>" {
			return nil, newSemaError(imp.Pos, "cannot resolve relative import %q without a file path", imp.Path)
		}

		targetPath := filepath.Clean(filepath.Join(curDir, filepath.FromSlash(imp.Path)))
		if l.state[targetPath] == 1 {
			targetID := l.canonicalID(targetPath)
			cycle := slices.Clone(l.stack)
			cycle = append(cycle, targetID)
			return nil, newSemaError(imp.Pos, "circular import detected: %s", strings.Join(cycle, " -> "))
		}

		if _, err := os.Stat(targetPath); err != nil {
			return nil, newSemaError(imp.Pos, "cannot import %q: %v", imp.Path, err)
		}

		depMod, err := l.visitFile(targetPath, nil, false)
		if err != nil {
			return nil, err
		}
		mod.Imports[alias] = depMod
	}

	l.state[key] = 2
	l.topoOrder = append(l.topoOrder, mod)
	return mod, nil
}

func (l *moduleLoader) getOrCreateStdlib(modPath string, pos token.Position) (*ModuleInfo, error) {
	if existing, ok := l.stdMods[modPath]; ok {
		return existing, nil
	}

	var mod *ModuleInfo
	switch modPath {
	case "std/fs":
		mod = buildStdFsModule()
	case "std/os":
		mod = buildStdOsModule()
	case "std/json":
		mod = buildStdJsonModule()
	case "std/cli":
		mod = buildStdCliModule()
	case "std/regex":
		mod = buildStdRegexModule()
	default:
		return nil, newSemaError(pos, "unknown standard library module %q", modPath)
	}

	l.stdMods[modPath] = mod
	l.byID[mod.ID] = mod
	l.topoOrder = append(l.topoOrder, mod)
	return mod, nil
}

func buildStdFsModule() *ModuleInfo {
	m := newModuleInfo("std/fs", "std/fs", false, true, nil)
	listStr := Type{Kind: KindList, ElemType: &StrType}

	m.Functions["readFile"] = &FnSymbol{
		Name:       "readFile",
		GoName:     "cbStdFsReadFile",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}},
		ReturnType: StrType,
		CanError:   true,
		Public:     true,
	}
	m.Functions["writeFile"] = &FnSymbol{
		Name:       "writeFile",
		GoName:     "cbStdFsWriteFile",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}, {Name: "content", Type: StrType}},
		ReturnType: VoidType,
		CanError:   true,
		Public:     true,
	}
	m.Functions["exists"] = &FnSymbol{
		Name:       "exists",
		GoName:     "cbStdFsExists",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}},
		ReturnType: BoolType,
		Public:     true,
	}
	m.Functions["mkdirAll"] = &FnSymbol{
		Name:       "mkdirAll",
		GoName:     "cbStdFsMkdirAll",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}},
		ReturnType: VoidType,
		CanError:   true,
		Public:     true,
	}
	m.Functions["listDir"] = &FnSymbol{
		Name:       "listDir",
		GoName:     "cbStdFsListDir",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}},
		ReturnType: listStr,
		CanError:   true,
		Public:     true,
	}
	m.Functions["isDir"] = &FnSymbol{
		Name:       "isDir",
		GoName:     "cbStdFsIsDir",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}},
		ReturnType: BoolType,
		Public:     true,
	}
	m.Functions["join"] = &FnSymbol{
		Name:       "join",
		GoName:     "cbStdFsJoin",
		Params:     []ParamSymbol{{Name: "a", Type: StrType}, {Name: "b", Type: StrType}},
		ReturnType: StrType,
		Public:     true,
	}
	m.Functions["base"] = &FnSymbol{
		Name:       "base",
		GoName:     "cbStdFsBase",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}},
		ReturnType: StrType,
		Public:     true,
	}
	m.Functions["dir"] = &FnSymbol{
		Name:       "dir",
		GoName:     "cbStdFsDir",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}},
		ReturnType: StrType,
		Public:     true,
	}
	m.Functions["ext"] = &FnSymbol{
		Name:       "ext",
		GoName:     "cbStdFsExt",
		Params:     []ParamSymbol{{Name: "path", Type: StrType}},
		ReturnType: StrType,
		Public:     true,
	}
	return m
}

func buildStdOsModule() *ModuleInfo {
	m := newModuleInfo("std/os", "std/os", false, true, nil)
	listStr := Type{Kind: KindList, ElemType: &StrType}

	execResult := &StructSymbol{
		Name:     "ExecResult",
		GoName:   "CbStdOsExecResult",
		ModuleID: "std/os",
		Public:   true,
		Fields: []FieldSymbol{
			{Name: "exitCode", Type: IntType},
			{Name: "stdout", Type: StrType},
			{Name: "stderr", Type: StrType},
		},
		FieldMap: map[string]FieldSymbol{
			"exitCode": {Name: "exitCode", Type: IntType},
			"stdout":   {Name: "stdout", Type: StrType},
			"stderr":   {Name: "stderr", Type: StrType},
		},
	}
	m.Structs["ExecResult"] = execResult
	execResultType := Type{Kind: KindStruct, Name: "ExecResult", ModuleID: "std/os", GoName: "CbStdOsExecResult"}

	m.Functions["args"] = &FnSymbol{
		Name:       "args",
		GoName:     "cbStdOsArgs",
		ReturnType: listStr,
		Public:     true,
	}
	m.Functions["exit"] = &FnSymbol{
		Name:       "exit",
		GoName:     "cbStdOsExit",
		Params:     []ParamSymbol{{Name: "code", Type: IntType}},
		ReturnType: VoidType,
		Public:     true,
	}
	m.Functions["getEnv"] = &FnSymbol{
		Name:       "getEnv",
		GoName:     "cbStdOsGetEnv",
		Params:     []ParamSymbol{{Name: "key", Type: StrType}},
		ReturnType: StrType,
		Public:     true,
	}
	m.Functions["exec"] = &FnSymbol{
		Name:       "exec",
		GoName:     "cbStdOsExec",
		Params:     []ParamSymbol{{Name: "cmd", Type: StrType}, {Name: "args", Type: listStr}},
		ReturnType: execResultType,
		CanError:   true,
		Public:     true,
	}
	m.Functions["readStdin"] = &FnSymbol{
		Name:       "readStdin",
		GoName:     "cbStdOsReadStdin",
		ReturnType: StrType,
		CanError:   true,
		Public:     true,
	}
	m.Functions["cwd"] = &FnSymbol{
		Name:       "cwd",
		GoName:     "cbStdOsCwd",
		ReturnType: StrType,
		CanError:   true,
		Public:     true,
	}
	return m
}

func buildStdJsonModule() *ModuleInfo {
	m := newModuleInfo("std/json", "std/json", false, true, nil)
	jvStruct := &StructSymbol{
		Name:     "JsonValue",
		GoName:   "CbStdJsonValue",
		ModuleID: "std/json",
		Public:   true,
		FieldMap: make(map[string]FieldSymbol),
	}
	m.Structs["JsonValue"] = jvStruct
	jvType := Type{Kind: KindStruct, Name: "JsonValue", ModuleID: "std/json", GoName: "CbStdJsonValue"}

	m.Functions["parse"] = &FnSymbol{
		Name:       "parse",
		GoName:     "cbStdJsonParse",
		Params:     []ParamSymbol{{Name: "raw", Type: StrType}},
		ReturnType: jvType,
		CanError:   true,
		Public:     true,
	}
	m.Functions["stringify"] = &FnSymbol{
		Name:       "stringify",
		GoName:     "cbStdJsonStringify",
		ReturnType: StrType,
		Public:     true,
	}
	return m
}

func buildStdCliModule() *ModuleInfo {
	m := newModuleInfo("std/cli", "std/cli", false, true, nil)
	listStr := Type{Kind: KindList, ElemType: &StrType}

	parserStruct := &StructSymbol{
		Name:     "Parser",
		GoName:   "*CbStdCliParser",
		ModuleID: "std/cli",
		Public:   true,
		FieldMap: make(map[string]FieldSymbol),
	}
	m.Structs["Parser"] = parserStruct
	parserType := Type{Kind: KindStruct, Name: "Parser", ModuleID: "std/cli", GoName: "*CbStdCliParser"}

	argsStruct := &StructSymbol{
		Name:     "ParsedArgs",
		GoName:   "CbStdCliArgs",
		ModuleID: "std/cli",
		Public:   true,
		Fields: []FieldSymbol{
			{Name: "helpRequested", Type: BoolType},
			{Name: "positionals", Type: listStr},
		},
		FieldMap: map[string]FieldSymbol{
			"helpRequested": {Name: "helpRequested", Type: BoolType},
			"positionals":   {Name: "positionals", Type: listStr},
		},
	}
	m.Structs["ParsedArgs"] = argsStruct

	m.Functions["newParser"] = &FnSymbol{
		Name:   "newParser",
		GoName: "cbStdCliNewParser",
		Params: []ParamSymbol{
			{Name: "name", Type: StrType},
			{Name: "version", Type: StrType},
			{Name: "description", Type: StrType},
		},
		ReturnType: parserType,
		Public:     true,
	}
	return m
}

func buildStdRegexModule() *ModuleInfo {
	m := newModuleInfo("std/regex", "std/regex", false, true, nil)

	regexStruct := &StructSymbol{
		Name:     "Regex",
		GoName:   "*CbStdRegex",
		ModuleID: "std/regex",
		Public:   true,
		FieldMap: make(map[string]FieldSymbol),
	}
	m.Structs["Regex"] = regexStruct
	regexType := Type{Kind: KindStruct, Name: "Regex", ModuleID: "std/regex", GoName: "*CbStdRegex"}

	m.Functions["compile"] = &FnSymbol{
		Name:       "compile",
		GoName:     "cbStdRegexCompile",
		Params:     []ParamSymbol{{Name: "pattern", Type: StrType}},
		ReturnType: regexType,
		CanError:   true,
		Public:     true,
	}
	m.Functions["match"] = &FnSymbol{
		Name:       "match",
		GoName:     "cbStdRegexMatch",
		Params:     []ParamSymbol{{Name: "pattern", Type: StrType}, {Name: "text", Type: StrType}},
		ReturnType: BoolType,
		CanError:   true,
		Public:     true,
	}
	return m
}
