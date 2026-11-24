package sema

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
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
	Program     *ast.Program
	Imports     map[string]*ModuleInfo
	Functions   map[string]*FnSymbol
	Structs     map[string]*StructSymbol
	Enums       map[string]*EnumSymbol
	GlobalVars  *Scope
}

func newModuleInfo(id, filePath string, isEntry bool, prog *ast.Program) *ModuleInfo {
	return &ModuleInfo{
		ID:          id,
		FilePath:    filePath,
		SanitizedID: sanitizeModuleID(id),
		IsEntry:     isEntry,
		Program:     prog,
		Imports:     make(map[string]*ModuleInfo),
		Functions:   make(map[string]*FnSymbol),
		Structs:     make(map[string]*StructSymbol),
		Enums:       make(map[string]*EnumSymbol),
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
	out := sb.String()
	if len(out) > 0 && out[0] >= '0' && out[0] <= '9' {
		return "_" + out
	}
	return out
}

func (m *ModuleInfo) symbolGoName(name string) string {
	if m.IsEntry {
		return name
	}
	return fmt.Sprintf("%s__%s", m.SanitizedID, name)
}

type moduleLoader struct {
	rootDir   string
	state     map[string]int // 0=unvisited, 1=visiting, 2=done
	modules   map[string]*ModuleInfo
	byID      map[string]*ModuleInfo
	topoOrder []*ModuleInfo
	stack     []string
}

func newModuleLoader(rootDir string) *moduleLoader {
	return &moduleLoader{
		rootDir: rootDir,
		state:   make(map[string]int),
		modules: make(map[string]*ModuleInfo),
		byID:    make(map[string]*ModuleInfo),
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
	mod, err := l.visitFile(absEntry, entryProg, true, "")
	if err != nil {
		return nil, nil, nil, err
	}
	return l.topoOrder, mod, l.byID, nil
}

func (l *moduleLoader) visitFile(absPath string, preParsed *ast.Program, isEntry bool, overrideID string) (*ModuleInfo, error) {
	key := absPath
	if key == "" {
		key = "<memory>"
	}
	modID := overrideID
	if modID == "" {
		if key == "<memory>" {
			modID = "main"
		} else {
			modID = l.canonicalID(key)
		}
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

	mod := newModuleInfo(modID, absPath, isEntry, prog)
	l.modules[key] = mod
	l.byID[modID] = mod

	curDir := "."
	if absPath != "" && absPath != "<memory>" {
		curDir = filepath.Dir(absPath)
	}

	for _, decl := range prog.Decls {
		imp, ok := decl.(*ast.ImportDecl)
		if !ok {
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

		depMod, err := l.visitFile(targetPath, nil, false, "")
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
	if existing, ok := l.byID[modPath]; ok {
		return existing, nil
	}

	stdFile, ok := findStdlibFile(l.rootDir, modPath)
	if !ok {
		return nil, newSemaError(pos, "unknown standard library module %q", modPath)
	}

	if l.state[stdFile] == 1 {
		cycle := slices.Clone(l.stack)
		cycle = append(cycle, modPath)
		return nil, newSemaError(pos, "circular import detected: %s", strings.Join(cycle, " -> "))
	}

	return l.visitFile(stdFile, nil, false, modPath)
}

func isValidStdlibRel(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.HasSuffix(rel, "/") || strings.ContainsRune(rel, '\\') {
		return false
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

func isProjectRootDir(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "bootstrap", "go.mod")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	return false
}

func findStdlibFile(rootDir, modPath string) (string, bool) {
	if !strings.HasPrefix(modPath, "std/") {
		return "", false
	}
	rel := strings.TrimPrefix(modPath, "std/")
	if !isValidStdlibRel(rel) {
		return "", false
	}
	relPath := filepath.FromSlash(rel) + ".cb"

	var candidates []string
	if envDir := os.Getenv("CB_STDLIB_DIR"); envDir != "" {
		candidates = append(candidates, envDir)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates, filepath.Join(exeDir, "std"), filepath.Join(exeDir, "..", "std"))
	}

	addAncestors := func(startDir string) {
		if startDir == "" {
			return
		}
		cur := filepath.Clean(startDir)
		for {
			if isProjectRootDir(cur) {
				candidates = append(candidates, filepath.Join(cur, "std"))
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	addAncestors(rootDir)
	if wd, err := os.Getwd(); err == nil {
		addAncestors(wd)
	}
	if _, callerFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(callerFile), "..", "..", "..", "std")))
	}

	seen := make(map[string]bool, len(candidates))
	for _, dir := range candidates {
		cleanDir := filepath.Clean(dir)
		if seen[cleanDir] {
			continue
		}
		seen[cleanDir] = true
		full := filepath.Join(cleanDir, relPath)
		relCheck, err := filepath.Rel(cleanDir, full)
		if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
			continue
		}
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, true
		}
	}
	return "", false
}
