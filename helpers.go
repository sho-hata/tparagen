package tparagen

import (
	"go/ast"
	"path/filepath"
	"strings"
	"sync"
)

// pkgID identifies a package by its directory and its name: the test files of
// a directory can belong to two packages, foo and foo_test.
type pkgID struct {
	dir, name string
}

// packageOf returns the package that the file f, read from filename, belongs to.
func packageOf(filename string, f *ast.File) pkgID {
	dir, err := filepath.Abs(filepath.Dir(filename))
	if err != nil {
		dir = filepath.Dir(filename)
	}

	return pkgID{dir: dir, name: f.Name.Name}
}

// helperIndex knows the helper functions that a test cannot call with its
// *testing.T once it is parallel: they call t.Setenv, t.Chdir or
// cryptotest.SetGlobalRandom(t, ...) on the testing value they are given,
// directly or through another such helper, and those calls panic in a
// parallel test.
type helperIndex struct {
	// funcs and methods hold the names of such functions and methods, by the
	// package that declares them.
	funcs, methods map[pkgID]map[string]bool
	// pkgsByPath maps an import path to the packages found in its directory.
	pkgsByPath map[string][]pkgID
}

// deniesParallel reports whether call hands one of tIdents to a helper that
// cannot be called from a parallel test. The call is made from package pkg,
// in a file importing imports; local reports whether a name is declared in
// the calling function, i.e. does not refer to an imported package.
func (h *helperIndex) deniesParallel(call *ast.CallExpr, pkg pkgID, imports map[string]string, local func(string) bool, tIdents map[string]bool) bool {
	if h == nil || !passesTestingValue(call, tIdents) {
		return false
	}

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return h.funcs[pkg][fun.Name]
	case *ast.SelectorExpr:
		x, ok := fun.X.(*ast.Ident)
		if !ok {
			return false
		}

		if path, ok := imports[x.Name]; ok && !local(x.Name) {
			for _, p := range h.pkgsByPath[path] {
				if h.funcs[p][fun.Sel.Name] {
					return true
				}
			}

			return false
		}

		// A method call such as s.setup(t). The type of s is unknown, so
		// any method of this package with that name counts.
		return h.methods[pkg][fun.Sel.Name]
	}

	return false
}

// passesTestingValue reports whether one of the arguments of call is one of
// tIdents.
func passesTestingValue(call *ast.CallExpr, tIdents map[string]bool) bool {
	for _, arg := range call.Args {
		if id, ok := arg.(*ast.Ident); ok && tIdents[id.Name] {
			return true
		}
	}

	return false
}

// helperCandidate is a function or method that takes a testing value, and so
// may be a helper that denies Parallel().
type helperCandidate struct {
	pkg     pkgID
	decl    *ast.FuncDecl
	imports map[string]string
	locals  map[string]bool
	tIdents map[string]bool
}

// helperIndexBuilder collects the helper candidates of many files, possibly
// concurrently, and then builds a helperIndex from them.
type helperIndexBuilder struct {
	mu         sync.Mutex
	candidates []helperCandidate
	pkgsByPath map[string][]pkgID
	seen       map[pkgID]bool
}

// addFile collects the helper candidates declared in f, which was read from
// filename. importPath is the import path of the file's directory, or "" if it
// is unknown.
func (b *helperIndexBuilder) addFile(filename string, f *ast.File, importPath string) {
	pkg := packageOf(filename, f)
	imports := importNames(f)

	var candidates []helperCandidate

	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}

		params := map[string]bool{}

		for _, field := range fd.Type.Params.List {
			if !isTestingValue(field.Type) {
				continue
			}

			for _, name := range field.Names {
				params[name.Name] = true
			}
		}

		if len(params) == 0 {
			continue
		}

		tIdents := testingTIdents("", fd.Body)
		delete(tIdents, "")

		for name := range params {
			tIdents[name] = true
		}

		candidates = append(candidates, helperCandidate{
			pkg:     pkg,
			decl:    fd,
			imports: imports,
			locals:  localNames(fd),
			tIdents: tIdents,
		})
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.candidates = append(b.candidates, candidates...)

	if importPath == "" || strings.HasSuffix(pkg.name, "_test") {
		return
	}

	if b.seen == nil {
		b.seen = map[pkgID]bool{}
		b.pkgsByPath = map[string][]pkgID{}
	}

	if !b.seen[pkg] {
		b.seen[pkg] = true
		b.pkgsByPath[importPath] = append(b.pkgsByPath[importPath], pkg)
	}
}

// build returns the index of the helpers that deny Parallel(). A helper that
// calls another one with its testing value denies it too, so the candidates
// are checked again until nothing changes.
func (b *helperIndexBuilder) build() *helperIndex {
	h := &helperIndex{
		funcs:      map[pkgID]map[string]bool{},
		methods:    map[pkgID]map[string]bool{},
		pkgsByPath: b.pkgsByPath,
	}

	denies := make([]bool, len(b.candidates))

	for changed := true; changed; {
		changed = false

		for i, c := range b.candidates {
			if denies[i] || !c.deniesParallel(h) {
				continue
			}

			denies[i], changed = true, true

			byName := h.funcs
			if c.decl.Recv != nil {
				byName = h.methods
			}

			if byName[c.pkg] == nil {
				byName[c.pkg] = map[string]bool{}
			}

			byName[c.pkg][c.decl.Name.Name] = true
		}
	}

	return h
}

// deniesParallel reports whether the candidate calls a method or function
// that panics in a parallel test, or a helper already known to h, on one of
// its testing values.
func (c *helperCandidate) deniesParallel(h *helperIndex) bool {
	var denies bool

	local := func(name string) bool { return c.locals[name] }

	ast.Inspect(c.decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !denies
		}

		if forbidsParallel(call, c.tIdents) || h.deniesParallel(call, c.pkg, c.imports, local, c.tIdents) {
			denies = true
		}

		return !denies
	})

	return denies
}

// isTestingValue reports whether expr is the type of a value that a test can
// pass its *testing.T as: *testing.T, *testing.B, *testing.F or testing.TB.
func isTestingValue(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != testMethodPackageType {
		return false
	}

	switch sel.Sel.Name {
	case "T", "B", "F", "TB":
		return true
	}

	return false
}
