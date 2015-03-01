// Tutorial - Write a Shell in Go - 28 Feb 2015 - Loic Nageleisen
//
// Let's be honest: this tutorial draws heavily from Stephen Brennen's
// "Write a Shell in C" tutorial, available here:
//
// http://stephen-brennan.com/2015/01/16/write-a-shell-in-c/
//
// Therefore, all the credit should go to him, as this is mostly the same
// tutorial, only Go-flavored. This is also an example of literate programming.

// In Go, a package is a collection of source files, and the main package is
// the one that produces an executable.

package main

// Imports allow to reference external packages. Here we only use packages of
// Go's standard library, and since we have only one file, a Go workspace is
// not even necessary.
//
// While we're at it, do yourself a favor and install gocode, gofmt and
// goimports, and take the time to integrate them properly in your favorite
// editor (which shouldn't take too long). As an example, I did not write a
// single of those import statements, goimports did it for me.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
)

// Basic lifetime of a Shell
//
// Let's look a shell from the top down. A shell does three main things in its
// lifetime.
//
// - Initialise: In this step, a typical shell would read and execute its
//   configuration files. These change aspects of the shell’s behavior.
// - Interpret: Next, the shell reads commands from stdin (which could be
//   interactive, or a file) and executes them.
// - Terminate: After its commands are executed, the shell executes any
//   shutdown commands, frees up any memory, and terminates.

// These steps are so general that they could apply to many programs, but we’re
// going to use them for the basis for our shell. Our shell will be so simple
// that there won’t be any configuration files, and there won’t be any shutdown
// command. So, we’ll just call the looping function and then terminate. But in
// terms of architecture, it’s important to keep in mind that the lifetime of
// the program is more than just looping.

func main() {
	// Load config files, if any.

	// Run command loop.
	shellLoop()

	// Perfom any shutdown/cleanup.

	os.Exit(0)
}

// Here you can see that I just came up with a function, shellLoop(), that will
// loop, interpreting commands. We’ll see the implementation of that next.

// Basic loop of a shell
//
// So we’ve taken care of how the program should start up. Now, for the basic
// program logic: what does the shell do during its loop? Well, a simple way to
// handle commands is with three steps:
//
// - Read: Read the command from standard input.
// - Parse: Separate the command string into a program and arguments.
// - Execute: Run the parsed command.
//
// Here, I’ll translate those ideas into code for shellLoop():

func shellLoop() {
	var line string
	var args []string
	var status int

	for {
		fmt.Printf("> ")
		line = shellReadLine()
		args = shellSplitLine(line)
		status = shellExecute(args)

		if status == 0 {
			break
		}
	}
}

// Let’s walk through the code. The first few lines are just declarations. The
// for loop can't be checking the status variable, as it can't execute once
// before checking its value, therefore we just break on condition a
// posteriori. Within the loop, we print a prompt, call a function to read a
// line, call a function to split the line into args, and execute the args.
// Note that we’re using a status variable returned by shellExecute() to
// determine when to exit.

// Reading a line
//
// Reading a line from stdin sounds so simple, but Go has no ready-made
// function for that. The result is something that feels slightly lower-level
// than what one might expect from a batteries-included language, yet is
// powerful enough to cover any case. In hindsight, this is typical of Go to
// empower its users with simple yet composable primitives instead of
// ready-made black boxes with many parameters.
//
// Indeed, the sad thing is that you don’t know ahead of time how much text a
// user will enter into their shell. You can’t simply allocate a slice and hope
// they don’t exceed it. Instead, you need to start with a slice, and if they
// do exceed it, reallocate with more space. Fortunately, Go's append does
// exactly that, and we can even start from a nil slice. An interesting read
// about slices and append that I encourage you to read lies here:
// http://blog.golang.org/slices
// This is a common pattern in Go, and we'll use it to implement
// shellReadLine()

func shellReadLine() string {
	var err error
	var line, data []byte
	isPrefix := true
	r := bufio.NewReader(os.Stdin)

	for isPrefix && err == nil {
		data, isPrefix, err = r.ReadLine()
		line = append(line, data...)
	}

	if err == io.EOF {
		fmt.Println()
	}

	return string(line)
}

// The first part is a lot of declarations and initialisations. If you hadn’t
// noticed, I prefer to keep the style of declaring variables before the rest
// of the code. I also favor declaring nil-valued vars first, then those whose
// type will be inferred from the assignment.
//
// The meat of the function is the for loop. In the loop, we read until a
// newline is reached, the bufio reader is filled (which sets isPrefix to
// true), or an error occurs (which sets err). The EOF error is special-cased
// as you want people typing ^D to have the shell exit. This is, in fact, not
// done here, and we simply print a newline to have the prompt displayed again
// properly.
//
// os.Stdin implements the source, bufio.Reader handles all the buffered
// reading magic, and append handles all the reallocations necessary to
// accomodate for the whole user input. Since the reader is unconcerned about
// encodings and returns raw bytes, we convert the line into a Unicode string
// right before returning. It does not look like much, but this is a prime
// example of composability at its best.

// Parsing the line
//
// OK, so if we look back at the loop, we see that we now have implemented
// shellReadline(), and we have the line of input. Now, we need to parse that
// line into a list of arguments. I’m going to make a glaring simplification
// here, and say that we won’t allow quoting or backslash escaping in our
// command line arguments. Instead, we will simply use whitespace to separate
// arguments from each other. So the command echo "this message" would not call
// echo with a single argument this message, but rather it would call echo with
// two arguments: '"this' and 'message"'.
//
// With those simplifications, all we need to do is split the string on
// whitespace. Fortunately, Go provides us with numerous string operations in
// the strings package.

func shellSplitLine(line string) []string {
	return strings.Fields(line)
}

// So, once all is said and done, we have an array of tokens, ready to execute.
// Which begs the question, how do we do that?

// How shells start processes
//
// Now, we’re really at the heart of what a shell does. Starting processes is
// the main function of shells. So writing a shell means that you need to know
// exactly what’s going on with processes and how they start. That’s why I’m
// going to take us on a short diversion to discuss processes in Unix.
//
// There are only two ways of starting processes on Unix. The first one (which
// almost doesn’t count) is by being Init. You see, when a Unix computer boots,
// its kernel is loaded. Once it is loaded and initialized, the kernel starts
// only one process, which is called Init. This process runs for the entire
// length of time that the computer is on, and it manages loading up the rest
// of the processes that you need for your computer to be useful.
//
// Since most programs aren’t Init, that leaves only one practical way for
// processes to get started: the fork() system call. When this function is
// called, the operating system makes a duplicate of the process and starts
// them both running. The original process is called the “parent”, and the new
// one is called the “child”. fork() returns 0 to the child process, and it
// returns to the parent the process ID number (PID) of its child. In essence,
// this means that the only way for new processes is to start is by an existing
// one duplicating itself.
//
// This might sound like a problem. Typically, when you want to run a new
// process, you don’t just want another copy of the same program – you want to
// run a different program. That’s what the exec() system call is all about. It
// replaces the current running program with an entirely new one. This means
// that when you call exec, the operating system stops your process, loads up
// the new program, and starts that one in its place. A process never returns
// from an exec() call (unless there’s an error).
//
// With these two system calls, we have the building blocks for how most
// programs are run on Unix. First, an existing process forks itself into two
// separate ones. Then, the child uses exec() to replace itself with a new
// program. The parent process can continue doing other things, and it can even
// keep tabs on its children, using the system call wait().
//
// Phew! That’s a lot of information, but with all that background we will be
// able to make sense of what happens behind the scenes of the following code
// for launching a program. I say behind the scenes because Go wants to provide
// an interface as robust and system agnostic as is pragmatically possible.

func shellLaunch(args []string) int {
	path, err := exec.LookPath(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	procAttr := os.ProcAttr{}
	procAttr.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}

	process, err := os.StartProcess(path, args, &procAttr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		for {
			state, err := process.Wait()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}

			waitStatus, ok := state.Sys().(syscall.WaitStatus)
			if ok && (waitStatus.Exited() || waitStatus.Signaled()) {
				break
			}
		}
	}

	return 1
}

// Alright. This function takes the list of arguments that we created earlier.
// Then, it looks to resolve the absolute path for the program we want to run
// via the os/exec package. This is because Go does not expose system-specific
// calls such as execvp that delegate this resolution to the OS, instead
// exposing a single os.StartProcess function that looks to work uniformly
// across OSes (including non-Unix ones) and provide compatibility between
// goroutines (which may map onto threads), forks and exec. The os/exec package
// also provides a more object-oriented (and more potent) exec.Cmd type, but
// it's not as useful to us right now as the low-level os.StartProcess, because
// we handle Wait() differently.
//
// In addition to the path to the program and the list of arguments,
// StartProcess also takes an argument containing additional attributes in
// order to offer as much functionality as the Unix exec family calls. An
// exec'd program may inherit the environment variables its spawner runs in
// (which is the default), or it can be passed an arbitrary environment.
// Additionally, Unix's fork will make every opened file descriptor readily
// available to the child as well as the parent, and a typical mistake in C is
// to forget to close the descriptors that won't be useful to the child. Go
// takes a friendlier approach and all descriptors will be closed unless they
// are passed on to the child in the attributes. Since we want our spawned
// program to interact with the user's terminal, we'd rather pass along Stdin,
// Stdout and Stderr. It is also possible to pass additional, system specific
// attributes, but beware of your code's portability.
//
// Once the process is started, we wait for the command to finish running. We
// use Wait() to wait for the process’s state to change. Unfortunately,
// Wait() returns a ProcessState that is system agnostic but quite light on
// information. Fortunately ProcessState is symmetric with ProcAttrs and can
// return a system-specific type, and since it comes from a generic function
// returning an interface{}, we need to do a type assertion. Processes can
// change state in lots of ways, and not all of them mean that the process has
// ended. A process can either exit (normally, or with an error code), or it
// can be killed by a signal. So, we use the methods of syscall.WaitStatus to
// wait until either the processes are exited or killed. Then, the function
// finally returns a 1, as a signal to the calling function that we should
// prompt for input again.

// Shell Builtins
//
// You may have noticed that the shellLoop() function calls shellExecute(), but
// above, we titled our function shellLaunch(). This was intentional! You see,
// most commands a shell executes are programs, but not all of them. Some of
// them are built right into the shell.
//
// The reason is actually pretty simple. If you want to change directory, you
// need to use the function Chdir(). The thing is, the current directory is a
// property of a process. So, if you wrote a program called cd that changed
// directory, it would just change its own current directory, and then
// terminate. Its parent process’s current directory would be unchanged.
// Instead, the shell process itself needs to execute Chdir(), so that its own
// current directory is updated. Then, when it launches child processes, they
// will inherit that directory too.
//
// Similarly, if there was a program named exit, it wouldn’t be able to exit
// the shell that called it. That command also needs to be built into the
// shell. Also, most shells are configured by running configuration scripts,
// like ~/.bashrc. Those scripts use commands that change the operation of the
// shell. These commands could only change the shell’s operation if they were
// implemented within the shell itself.
//
// So, it makes sense that we need to add some commands to the shell itself.
// The ones I added to my shell are cd, pwd, exit, and help. Here are their
// function implementations below:

var shellBuiltIn map[string](func([]string) int)

func init() {
	shellBuiltIn = map[string](func([]string) int){
		"cd":   shellCd,
		"pwd":  shellPwd,
		"exit": shellExit,
		"help": shellHelp,
	}
}

func shellCd(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "cd: missing argument")
	} else if err := os.Chdir(args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	return 1
}

func shellPwd(args []string) int {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	fmt.Println(dir)
	return 1
}

func shellHelp(args []string) int {
	var keys []string
	for k := range shellBuiltIn {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		fmt.Println(name)
	}
	return 1
}

func shellExit(args []string) int {
	return 0
}

// There are three parts to this code. The first part is declaring the map that
// will resolve our builtin names to their functions. This is so that, in the
// future, builtin commands can be added simply by modifying this map, rather
// than editing a large switch statement somewhere in the code.
//
// While tempting, initialising shellBuiltIn with a literal would cause a
// compile-time error because of an initialisation loop with shellHelp. This is
// where the second part comes into play: we delegate our map initialisation to
// the package's init function, which is called even before the main function
// is called. Be careful though, as if your package spans multiple Go source
// files, their init() function has no execution ordering guarantee.
//
// Finally, I implement each function. The shellCd() function first checks that
// its second argument exists, and prints an error message if it doesn’t. Then,
// it calls os.Chdir(), checks for errors, and returns. The help function
// prints a nice message and the names of all the builtins. The pwd function
// prints the current working directory, and the exit function returns 0, as a
// signal for the command loop to terminate.
//
// Using a map has an interesting effect, obvious to anyone used to similar
// hash table data structures: the ordering is not guaranteed. Go goes as far
// as voluntarily ensuring iterating over a map's range yields a random
// ordering so that nobody relies on a providential ordering due to a
// particular runtime implementation. Once copied, sorting the keys
// alphabetically is done in place by the sort package, with a ready-made
// string sorter.

// Putting together builtins and processes
//
// The last missing piece of the puzzle is to implement shellExecute(), the
// function that will either launch either a builtin, or a process. If you’re
// reading this far, you’ll know that we’ve set ourselves up for a really
// simple function:

func shellExecute(args []string) int {
	if len(args) == 0 {
		return 1
	}

	if builtIn := shellBuiltIn[args[0]]; builtIn != nil {
		return builtIn(args)
	}

	return shellLaunch(args)
}

// All this does is check if the command exists as a builtin, and if so, run
// it. If it doesn’t match a builtin, it calls shellLaunch() to launch the
// process. The one caveat is that args might just contain an empty array, if
// the user entered an empty string, or just whitespace. So, we need to check
// for that case at the beginning.

// Putting it all together
//
// That’s all the code that goes into the shell. If you’ve read along, you
// should understand completely how the shell works. To try it out (on a Unix
// machine), just `go run` this file!

// Wrap up
//
// If you read this and wondered how in the world I knew how to use those
// packages, the answer is simple: godoc. And while golang.org is readily
// available and histong godoc, running godoc locally from your workspace will
// include documentation from any package you have installed. If you know what
// you’re looking for, and you just want to know how to use it, godoc is your
// best friend. If you want to dig a bit deeper, you will want to dive into the
// source code, which is both incredibly clean, throughtly documented and
// readily available straight from godoc too! And remember, godoc works both
// like `man` in a terminal as well as serving a browsable documentation from
// the comfort of your browser.
//
// Obviously, this shell isn’t feature-rich. Some of its more glaring omissions
// are:
//
// - Only whitespace separating arguments, no quoting or backslash escaping.
// - No piping or redirection.
// - Few standard builtins.
// - No globbing.
//
// The implementation of all of this stuff is really interesting, but way more
// than I could ever fit into an article like this. If I ever get around to
// implementing any of them, I’ll be sure to write a follow-up about it. But
// I’d encourage any reader to try implementing this stuff yourself. If you’re
// met with success, drop me a line in the comments below, I’d love to see the
// code.
//
// And finally, thanks for reading this tutorial (if anyone did). I enjoyed
// writing it, and I hope you enjoyed reading it. Let me know what you think!
