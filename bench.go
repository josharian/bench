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

	testRun   = flag.String("run", ".", "-test=")
	testBench = flag.String("bench", ".", "-bench=")
	sleep     = flag.Duration("sleep", 0, "time to sleep between benchmark runs")
	// testBenchmem = flag.Bool("benchmem", false, "-benchmem") // TODO
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
	defer os.RemoveAll(tempdir)
	printf(0, "Using temp dir %v", tempdir)

	printf(0, "Compiling before.test")
	beforeTest := compileTest(*gorootBefore, "before.test")

	printf(0, "Compiling after.test")
	afterTest := compileTest(*gorootAfter, "after.test")

	printf(0, "Running before.test")
	runTest(beforeTest, "-test.run="+*testRun)

	printf(0, "Running after.test")
	runTest(afterTest, "-test.run="+*testRun)

	printf(0, "Elapsed: %v\n", time.Now().Sub(start))

	var beforeBench string
	var afterBench string
	var n int

	start = time.Now()
	for {
		printf(1, "Running before benchmarks")
		beforeBench += runTest(beforeTest, "-test.run=NONE", "-test.bench="+*testBench)
		time.Sleep(*sleep)

		printf(1, "Running after benchmarks")
		afterBench += runTest(afterTest, "-test.run=NONE", "-test.bench="+*testBench)

		n++
		printf(0, "--- %d iter (%v each):", n, time.Now().Sub(start)/time.Duration(n))
		out := benchcmp(beforeBench, afterBench)

		printf(0, "%s\n\n", out)

		time.Sleep(*sleep)
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

func benchcmp(before, after string) string {
	beforef := writetemp("before.bench", before)
	afterf := writetemp("after.bench", after)
	cmd := exec.Command("benchcmp", "-mag", "-best", beforef, afterf)
	return runCmd(cmd)
}

func runTest(test string, args ...string) string {
	if *verbose > 1 {
		args = append([]string{"-test.v"}, args...)
	}
	cmd := exec.Command(test, args...)
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

func compileTest(goroot, filename string) string {
	test := filepath.Join(tempdir, filename)
	args := []string{"test", "-c", "-o", test}
	args = append(args, flag.Args()...)
	cmd := exec.Command("bin/go", args...)
	cmd.Dir = goroot
	cmd.Env = []string{"GOROOT=" + goroot}
	runCmd(cmd)
	return test
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
