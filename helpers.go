package tparagen

import (
	"bytes"
	"go/ast"
	"path/filepath"
	"regexp"
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

// fileImports is how a file refers to the packages it imports.
type fileImports struct {
	// named maps the name a package is referred to by to its import path:
	// the explicit name, or for an unnamed import a guess from the path.
	named map[string]string
	// unnamed are the import paths imported without a name. The package
	// name, which need not match the path, is known only from the index.
	unnamed []string
	// dot are the import paths imported with ".".
	dot []string
	// testing is the name the file refers to the testing package by, or ""
	// when it does not import it.
	testing string
}

func newFileImports(f *ast.File) *fileImports {
	imp := &fileImports{named: importNames(f)}

	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, `"`)

		switch {
		case spec.Name == nil:
			imp.unnamed = append(imp.unnamed, path)
		case spec.Name.Name == ".":
			imp.dot = append(imp.dot, path)
		}
	}

	for name, path := range imp.named {
		if path == "testing" {
			imp.testing = name
		}
	}

	return imp
}

// callee is a call to a function or method which may be a helper.
type callee struct {
	name string
	// selector is set for x.name(...) calls, recv is x when it is an
	// identifier, and local reports whether x is declared in the caller (so
	// it is a variable, not an imported package).
	selector bool
	recv     string
	local    bool
	// passes reports whether the call hands a testing value to the callee.
	// A helper that reaches Setenv through a stored testing value (see
	// helperIndex.stored) denies Parallel() whether or not one is passed.
	passes bool
}

// calleeOf describes the function or method that call calls. local reports
// whether a name is declared in the calling function.
func calleeOf(call *ast.CallExpr, local func(string) bool) (callee, bool) {
	fun := call.Fun

	// A generic helper called with explicit type arguments: f[int](t).
	switch f := fun.(type) {
	case *ast.IndexExpr:
		fun = f.X
	case *ast.IndexListExpr:
		fun = f.X
	}

	switch fun := fun.(type) {
	case *ast.Ident:
		return callee{name: fun.Name}, true
	case *ast.SelectorExpr:
		c := callee{name: fun.Sel.Name, selector: true}

		if x, ok := fun.X.(*ast.Ident); ok {
			c.recv, c.local = x.Name, local(x.Name)
		}

		return c, true
	}

	return callee{}, false
}

// helperIndex knows the helper functions that a test cannot call once it is
// parallel: they call t.Setenv, t.Chdir or cryptotest.SetGlobalRandom(t, ...)
// on the testing value they are given, directly or through another such
// helper, or on a testing value stored in a struct (`e.t.Setenv(...)` in a
// method of `type env struct{ t *testing.T }`), and those calls panic in a
// parallel test.
type helperIndex struct {
	// funcs and methods hold the names of the helpers taking a testing
	// value, by the package that declares them. They deny Parallel() when
	// called with one.
	funcs, methods map[pkgID]map[string]bool
	// stored and storedMethods hold the names of the helpers reaching
	// Setenv or Chdir on a stored testing value. They take none, so they
	// deny Parallel() whenever they are called.
	stored, storedMethods map[pkgID]map[string]bool
	// pkgsByPath maps an import path to the packages found in its directory.
	pkgsByPath map[string][]pkgID
}

// deniesParallel reports whether call is a call to a helper that cannot be
// made from a parallel test: one of tIdents is handed to a helper taking a
// testing value, or a helper reaching Setenv on a stored one is called. The
// call is made from package pkg, in a file importing imp; local reports
// whether a name is declared in the calling function, i.e. does not refer to
// an imported package.
func (h *helperIndex) deniesParallel(call *ast.CallExpr, pkg pkgID, imp *fileImports, local func(string) bool, tIdents map[string]bool) bool {
	if h == nil {
		return false
	}

	c, ok := calleeOf(call, local)
	c.passes = passesTestingValue(call, tIdents)

	return ok && h.denies(c, pkg, imp)
}

// denies reports whether c, called from package pkg in a file importing imp,
// is a known helper: one taking a testing value when c passes one, or one
// reaching Setenv on a stored testing value in any case. The types of values
// are unknown, so a method call counts when any method of that name in this
// package or an imported one is a helper.
func (h *helperIndex) denies(c callee, pkg pkgID, imp *fileImports) bool {
	if c.passes && h.deniesIn(c, pkg, imp, h.funcs, h.methods) {
		return true
	}

	return h.deniesIn(c, pkg, imp, h.stored, h.storedMethods)
}

// deniesIn is denies looking up c in the given function and method sets.
func (h *helperIndex) deniesIn(c callee, pkg pkgID, imp *fileImports, funcs, methods map[pkgID]map[string]bool) bool {
	if !c.selector {
		if funcs[pkg][c.name] {
			return true
		}

		for _, path := range imp.dot {
			if anyFunc(funcs, h.pkgsByPath[path], c.name) {
				return true
			}
		}

		return false
	}

	if c.recv != "" && !c.local {
		if pkgs := h.importedAs(c.recv, imp); len(pkgs) > 0 {
			return anyFunc(funcs, pkgs, c.name)
		}
	}

	if methods[pkg][c.name] {
		return true
	}

	for _, path := range imp.paths() {
		for _, p := range h.pkgsByPath[path] {
			if methods[p][c.name] {
				return true
			}
		}
	}

	return false
}

// importedAs returns the packages that the file importing imp refers to by
// name.
func (h *helperIndex) importedAs(name string, imp *fileImports) []pkgID {
	var pkgs []pkgID

	if path, ok := imp.named[name]; ok {
		pkgs = append(pkgs, h.pkgsByPath[path]...)
	}

	for _, path := range imp.unnamed {
		for _, p := range h.pkgsByPath[path] {
			if p.name == name {
				pkgs = append(pkgs, p)
			}
		}
	}

	return pkgs
}

func anyFunc(funcs map[pkgID]map[string]bool, pkgs []pkgID, name string) bool {
	for _, p := range pkgs {
		if funcs[p][name] {
			return true
		}
	}

	return false
}

// paths returns every import path of the file.
func (imp *fileImports) paths() []string {
	paths := make([]string, 0, len(imp.named)+len(imp.dot))

	for _, path := range imp.named {
		paths = append(paths, path)
	}

	return append(paths, imp.dot...)
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

// helperSummary is what the index needs to know about a function or method
// that takes a testing value, or reaches Setenv or Chdir on a stored one.
// Keeping this instead of the syntax tree lets the files be dropped as soon
// as they are scanned.
type helperSummary struct {
	pkg     pkgID
	name    string
	method  bool
	imports *fileImports
	// takesTesting is set when the function takes a testing value. One that
	// does not is only summarized when it denies by itself (see stored in
	// helperIndex): its callees are not followed.
	takesTesting bool
	// denies is set when the function itself calls Setenv, Chdir or
	// SetGlobalRandom on one of its testing values, or Setenv or Chdir on a
	// testing value stored in one of its variables (`e.t.Setenv(...)`).
	denies bool
	// callees are the calls to other functions or methods that may be
	// helpers: those handing one of its testing values to the callee, and
	// those on a value of the function (`e.set()`) which may reach Setenv
	// on a stored testing value.
	callees []callee
}

// helperIndexBuilder collects the helper summaries of many files, possibly
// concurrently, and then builds a helperIndex from them.
type helperIndexBuilder struct {
	mu         sync.Mutex
	summaries  []helperSummary
	pkgsByPath map[string][]pkgID
	seen       map[pkgID]bool
}

// addFile collects the helper summaries of the functions declared in f, which
// was read from filename. importPath is the import path of the file's
// directory, or "" if it is unknown.
func (b *helperIndexBuilder) addFile(filename string, f *ast.File, importPath string) {
	pkg := packageOf(filename, f)
	imports := newFileImports(f)

	var summaries []helperSummary

	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil || isEntryPoint(fd) {
			continue
		}

		if s, ok := summarize(fd, pkg, imports); ok {
			summaries = append(summaries, s)
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.summaries = append(b.summaries, summaries...)

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

// isEntryPoint reports whether fd is run by `go test` itself, so no other
// function calls it as a helper.
func isEntryPoint(fd *ast.FuncDecl) bool {
	if fd.Recv != nil {
		return false
	}

	for _, prefix := range []string{"Test", "Benchmark", "Fuzz"} {
		if strings.HasPrefix(fd.Name.Name, prefix) {
			return true
		}
	}

	return false
}

// summarize returns the summary of fd, or false when it neither takes a
// testing value nor reaches Setenv or Chdir on a stored one.
func summarize(fd *ast.FuncDecl, pkg pkgID, imports *fileImports) (helperSummary, bool) {
	params := map[string]bool{}

	for _, field := range fd.Type.Params.List {
		if !isTestingValue(field.Type, imports.testing) {
			continue
		}

		for _, name := range field.Names {
			params[name.Name] = true
		}
	}

	tIdents := testingTIdents("", fd.Body)
	delete(tIdents, "")

	for name := range params {
		tIdents[name] = true
	}

	locals := localNames(fd)
	local := func(name string) bool { return locals[name] }

	s := helperSummary{pkg: pkg, name: fd.Name.Name, method: fd.Recv != nil, imports: imports, takesTesting: len(params) > 0}

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || s.denies {
			return !s.denies
		}

		if forbidsParallel(call, tIdents, local) {
			s.denies = true

			return false
		}

		if !s.takesTesting {
			return true
		}

		if c, ok := calleeOf(call, local); ok {
			c.passes = passesTestingValue(call, tIdents)

			// A plain function cannot reach a stored testing value unless it
			// is given one; a method or a function value stored in a local
			// value can (`e.set()`, `tc.setup()`).
			if c.passes || (c.selector && c.local) {
				s.callees = append(s.callees, c)
			}
		}

		return true
	})

	if !s.takesTesting && !s.denies {
		return helperSummary{}, false
	}

	return s, true
}

// build returns the index of the helpers that deny Parallel(). A helper that
// calls another one with its testing value denies it too, so the summaries
// are checked again until nothing changes.
func (b *helperIndexBuilder) build() *helperIndex {
	h := &helperIndex{
		funcs:         map[pkgID]map[string]bool{},
		methods:       map[pkgID]map[string]bool{},
		stored:        map[pkgID]map[string]bool{},
		storedMethods: map[pkgID]map[string]bool{},
		pkgsByPath:    b.pkgsByPath,
	}

	denies := make([]bool, len(b.summaries))

	for changed := true; changed; {
		changed = false

		for i, s := range b.summaries {
			if denies[i] || !s.deniesParallel(h) {
				continue
			}

			denies[i], changed = true, true

			byName := h.funcs

			switch {
			case !s.takesTesting && s.method:
				byName = h.storedMethods
			case !s.takesTesting:
				byName = h.stored
			case s.method:
				byName = h.methods
			}

			if byName[s.pkg] == nil {
				byName[s.pkg] = map[string]bool{}
			}

			byName[s.pkg][s.name] = true
		}
	}

	return h
}

// deniesParallel reports whether the function calls a method or function that
// panics in a parallel test, or a helper already known to h, on one of its
// testing values.
func (s *helperSummary) deniesParallel(h *helperIndex) bool {
	if s.denies {
		return true
	}

	for _, c := range s.callees {
		if h.denies(c, s.pkg, s.imports) {
			return true
		}
	}

	return false
}

// isTestingValue reports whether expr is the type of a value that a test can
// pass its *testing.T as: *testing.T, *testing.B, *testing.F or testing.TB,
// where testing is the name the file refers to the testing package by.
func isTestingValue(expr ast.Expr, testing string) bool {
	if testing == "" {
		return false
	}

	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != testing {
		return false
	}

	switch sel.Sel.Name {
	case "T", "B", "F", "TB":
		return true
	}

	return false
}

var (
	funcDeclStart = regexp.MustCompile(`(?m)^func\b`)
	funcName      = regexp.MustCompile(`^func\s*(?:\([^)]*\)\s*)?([\pL_][\pL\pN_]*)`)
	testingParam  = regexp.MustCompile(`\.(?:T|B|F|TB)\b`)
	// storedDenyingCall matches a Setenv or Chdir call on a selector such as
	// e.t.Setenv(...): a testing value stored in a struct.
	storedDenyingCall = regexp.MustCompile(`\.[\pL_][\pL\pN_]*\.(?:Setenv|Chdir)\(`)
)

// mayStoreTestingValue reports whether src may declare a function that
// reaches Setenv or Chdir on a testing value stored in a struct. It looks at
// the text, so it can only err towards true.
func mayStoreTestingValue(src []byte) bool {
	return storedDenyingCall.Match(src)
}

// mayDeclareHelper reports whether src may declare a function, other than a
// test, benchmark or fuzz test, that takes a testing value. It looks at the
// text of each declaration up to its body, so it can only err towards true.
func mayDeclareHelper(src []byte) bool {
	for _, loc := range funcDeclStart.FindAllIndex(src, -1) {
		header := src[loc[0]:]
		if end := bytes.IndexByte(header, '{'); end >= 0 {
			header = header[:end]
		}

		if !testingParam.Match(header) {
			continue
		}

		m := funcName.FindSubmatch(header)
		if m == nil || !isEntryPoint(&ast.FuncDecl{Name: ast.NewIdent(string(m[1]))}) || bytes.HasPrefix(bytes.TrimPrefix(header, []byte("func")), []byte(" (")) {
			return true
		}
	}

	return false
}
