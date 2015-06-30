package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	verbose *count
	tempdir string

	gorootBefore = flag.String("before", "/Users/jbleechersnyder/src/go-cmp", "GOROOT for 'before'")
	gorootAfter  = flag.String("after", "/Users/jbleechersnyder/src/go", "GOROOT for 'after'")

	testRun       = flag.String("run", "NONE", "-test=")
	testBench     = flag.String("bench", ".", "-bench=")
	testBenchmem  = flag.Bool("benchmem", false, "-benchmem=")
	testBenchtime = flag.Duration("benchtime", time.Second, "-benchtime=")
	testLdflags   = flag.String("ldflags", "", "-ldflags=")
	testGcflags   = flag.String("gcflags", "", "-gcflags=")

	sleep = flag.Duration("sleep", 0, "time to sleep between benchmark runs")
)

func main() {
	verbose = new(count)
	flag.Var(verbose, "v", "verbose")
	flag.Parse()

	start := time.Now()
	if flag.NArg() == 0 {
		// TODO: Better error message, print usage, etc.
		die("must provide at least one package")
	}

	printf(0, "Before: %v (%v)", *gorootBefore, version(*gorootBefore))
	printf(0, "After: %v (%v)", *gorootAfter, version(*gorootAfter))

	pkgs := flag.Args()

	if pkgs[0] == "std" {
		// TODO: Any reason not to do this? Assumption is that the package list always grows.
		pkgs = listStd(*gorootBefore)
	}

	var err error
	tempdir, err = ioutil.TempDir("", "bench")
	if err != nil {
		die(err)
	}
	printf(0, "Using temp dir %v", tempdir)

	printf(0, "Compiling before tests")
	beforeTests := compileTests(*gorootBefore, "before", pkgs)

	printf(0, "Compiling after tests")
	afterTests := compileTests(*gorootAfter, "after", pkgs)

	printf(0, "Running before tests")
	for _, test := range beforeTests {
		test.run("-test.run=" + *testRun)
	}

	printf(0, "Running after tests")
	for _, test := range afterTests {
		test.run("-test.run=" + *testRun)
	}

	printf(0, "Elapsed: %v\n", time.Now().Sub(start))

	beforeBenches := make(map[string]string)
	afterBenches := make(map[string]string)
	var n int

	for {
		n++
		for i := range pkgs {
			pkg := pkgs[i]

			beforeTest, ok := beforeTests[pkg]
			if !ok {
				continue
			}

			afterTest, ok := afterTests[pkg]
			if !ok {
				continue
			}

			start = time.Now()
			printf(1, "Running before benchmarks: %s", pkg)
			beforeBenches[pkg] += "\n"
			beforeBenches[pkg] += beforeTest.run("-test.run=NONE", "-test.bench="+*testBench, "-test.benchmem="+benchmemString(), "-test.benchtime="+testBenchtime.String())
			beforeBenches[pkg] += "\n"
			time.Sleep(*sleep)

			printf(1, "Running after benchmarks: %s", pkg)
			afterBenches[pkg] += "\n"
			afterBenches[pkg] += afterTest.run("-test.run=NONE", "-test.bench="+*testBench, "-test.benchmem="+benchmemString(), "-test.benchtime="+testBenchtime.String())
			afterBenches[pkg] += "\n"

			if beforeBenches[pkg] == "PASS" && afterBenches[pkg] == "PASS" {
				continue
			}

			printf(0, "--- %s, %d iter (%v)", pkg, n, time.Now().Sub(start))
			out := benchcmp(pkg, beforeBenches[pkg], afterBenches[pkg])
			printf(0, "%s\n\n", out)
			time.Sleep(*sleep)
		}

		if len(pkgs) > 1 {
			printf(0, "--- ALL, %d iter", n)
			var beforeAll string
			var afterAll string
			for _, pkg := range pkgs {
				beforeAll += beforeBenches[pkg]
				afterAll += afterBenches[pkg]
			}
			out := benchcmp("all", beforeAll, afterAll)
			lines := strings.Split(out, "\n")
			nlines := 50
			if len(lines) < nlines {
				nlines = len(lines) - 1
			}
			for _, line := range lines[:nlines] {
				printf(0, "%s", line)
			}
			printf(0, "\n\n")
		}
	}
}

func writetemp(filename, data string) string {
	f := filepath.Join(tempdir, filename)
	err := ioutil.WriteFile(f, []byte(data), 0644)
	if err != nil {
		dief("could not write %v: %v", f, err)
	}
	return f
}

func benchcmp(pkg, before, after string) string {
	pkg = strings.Replace(pkg, "/", "-", -1)
	beforef := writetemp("before-"+pkg+".bench", before)
	afterf := writetemp("after-"+pkg+".bench", after)
	cmd := exec.Command("benchstat", beforef, afterf)
	// don't die on errors, so don't use runCmd
	printf(1, "Running %v", commandString(cmd))
	out, _ := cmd.CombinedOutput()
	printf(2, "%s", out)
	return strings.TrimSpace(string(out))
}

func (t compiledTest) run(args ...string) string {
	if *verbose > 1 {
		args = append([]string{"-test.v"}, args...)
	}
	cmd := exec.Command(t.binary, args...)
	cmd.Dir = t.dir
	return runCmd(cmd)
}

func runCmd(cmd *exec.Cmd) string {
	printf(1, "Running %v", commandString(cmd))
	out, err := cmd.CombinedOutput()
	if err != nil {
		dief("%v failed (%v):\n%s", commandString(cmd), err, out)
	}
	printf(2, "%s", out)
	return strings.TrimSpace(string(out))
}

func commandString(cmd *exec.Cmd) string {
	if len(cmd.Args) == 0 {
		return cmd.Path
	}
	if cmd.Args[0] != cmd.Path {
		return strings.Join(append([]string{cmd.Path}, cmd.Args[1:]...), " ")
	}
	return strings.Join(cmd.Args, " ")
}

func version(goroot string) string {
	cmd := exec.Command("bin/go", "version")
	cmd.Dir = goroot
	return runCmd(cmd)
}

func listStd(goroot string) []string {
	cmd := exec.Command("bin/go", "list", "std")
	cmd.Dir = goroot
	out := runCmd(cmd)

	var pkgs []string
	for _, pkg := range strings.Split(out, "\n") {
		if !strings.HasPrefix(pkg, "cmd/") {
			pkgs = append(pkgs, string(pkg))
		}
	}
	return pkgs
}

func pkgDir(goroot, pkg string) string {
	cmd := exec.Command("bin/go", "list", "-f", "{{.Dir}}", pkg)
	cmd.Dir = goroot
	return runCmd(cmd)
}

type compiledTest struct {
	binary string
	dir    string
}

func compileTests(goroot, prefix string, pkgs []string) map[string]compiledTest {
	m := make(map[string]compiledTest)
	for _, pkg := range pkgs {
		filename := prefix + "-" + strings.Replace(pkg, "/", "-", -1) + ".test"
		path := filepath.Join(tempdir, filename)
		cmd := exec.Command("bin/go", "test", "-c", "-ldflags="+*testLdflags, "-gcflags="+*testGcflags, "-o", path, pkg)
		cmd.Dir = goroot
		cmd.Env = []string{
			"GOROOT=" + goroot,
			"PATH=" + os.Getenv("PATH"),
			"GOPATH=" + os.Getenv("GOPATH"),
		}
		runCmd(cmd)
		// If there is no test file, don't claim that there is one
		_, err := os.Stat(path)
		if err != nil {
			continue
		}
		m[pkg] = compiledTest{binary: path, dir: pkgDir(goroot, pkg)}
	}
	return m
}

func printf(thresh int, format string, v ...interface{}) {
	if int(*verbose) >= thresh {
		fmt.Printf(format, v...)
		fmt.Println()
	}
}

func die(v ...interface{}) {
	fmt.Fprintln(os.Stderr, v...)
	os.Exit(1)
}

func dief(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
	fmt.Fprintln(os.Stderr)
	os.Exit(1)
}

// The below is stolen from git-codereview.
// TODO: Extract to separate, re-usable package?
// TODO: Petition to get a counting flag added to the stdlib?

// count is a flag.Value that is like a flag.Bool and a flag.Int.
// If used as -name, it increments the count, but -name=x sets the count.
// Used for verbose flag -v.
type count int

func (c *count) String() string {
	return fmt.Sprint(int(*c))
}

func (c *count) Set(s string) error {
	switch s {
	case "true":
		*c++
	case "false":
		*c = 0
	default:
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid count %q", s)
		}
		*c = count(n)
	}
	return nil
}

func (c *count) IsBoolFlag() bool {
	return true
}

func benchmemString() string {
	if *testBenchmem {
		return "true"
	} else {
		return "false"
	}
}
