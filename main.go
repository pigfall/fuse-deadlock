package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	_ "unsafe"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

func main() {
	var backendDir string
	var mp string
	var runFuseInChildProcess bool
	flag.StringVar(&backendDir, "backend", "", "backend directory")
	flag.StringVar(&mp, "mountpoint", "", "mounpoint path")
	flag.BoolVar(&runFuseInChildProcess, "run-fuse-in-child-process", false, "Run fuse in child process")
	flag.Parse()

	if backendDir == "" {
		panic("must provide backend directory")
	}
	if mp == "" {
		panic("must provide mp")
	}

	if runFuseInChildProcess {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

		// The process is the init process. Reap the children processes.
		go func() {
			for {
				time.Sleep(time.Second)
				ReapChild()
			}
		}()

		execPath, err := os.Executable()
		if err != nil {
			panic(err)
		}

		// Run fuse in a child process.
		cmd := exec.Command(
			execPath,
			"--mountpoint="+mp,
			"--backend="+backendDir,
		)
		cmd.Env = append(os.Environ(), "FUSE_FS=test")
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		cmd.Process.Release()
		fmt.Println("Runed fuse as child process")
		<-sig
		cmd.Process.Kill()
		return
	}

	rootNode, err := NewLoopbackRoot(backendDir)
	if err != nil {
		panic(err)
	}

	sec := time.Second
	opts := &fs.Options{
		// The timeout options are to be compatible with libfuse defaults,
		// making benchmarking easier.
		AttrTimeout:  &sec,
		EntryTimeout: &sec,

		NullPermissions: true, // Leave file permissions on "000" files as-is

		MountOptions: fuse.MountOptions{
			AllowOther:        false,
			Debug:             false,
			DirectMount:       true,
			DirectMountStrict: true,
			FsName:            "fs",
			Name:              "loopback",
		},
	}

	rawFS := fs.NewNodeFS(rootNode, opts)
	server, err := fuse.NewServer(rawFS, mp, &opts.MountOptions)
	if err != nil {
		panic(err)
	}

	go server.Serve()
	if err := server.WaitMount(); err != nil {
		panic(err)
	}
	fmt.Println("Mounted")

	go func() {
		time.Sleep(time.Second * 5)
		f := filepath.Join(mp, "test")
		c := exec.Command("cat", f)
		fmt.Println("start to cat file %s", f)
		c.Run()
		fmt.Println("cat done: ", err)
	}()

	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-exitCh
		fmt.Println("Unmouting")
		server.Unmount()
	}()

	server.Wait()
}

var _ = (fs.FileReader)((*delayFileReader)(nil))

type loopbackNode struct {
	fs.LoopbackNode
}

type delayFileReader struct {
	fs.FileHandle
}

// Override the Open.
func (n *loopbackNode) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	fmt.Println("opening")
	time.Sleep(time.Hour)
	fmt.Println("open done")
	fh, fuseFlags, errno = n.LoopbackNode.Open(ctx, flags)
	return &delayFileReader{FileHandle: fh}, fuseFlags, errno
}

func (r *delayFileReader) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	return r.FileHandle.(fs.FileReader).Read(ctx, dest, off)
}

func NewLoopbackRoot(rootPath string) (*loopbackNode, error) {
	var st syscall.Stat_t
	err := syscall.Stat(rootPath, &st)
	if err != nil {
		return nil, err
	}

	root := &fs.LoopbackRoot{
		Path: rootPath,
		Dev:  uint64(st.Dev),
	}

	root.NewNode = func(rootData *fs.LoopbackRoot, parent *fs.Inode, name string, st *syscall.Stat_t) fs.InodeEmbedder {
		return &loopbackNode{
			LoopbackNode: fs.LoopbackNode{
				RootData: rootData,
			},
		}
	}

	rootNode := root.NewNode(root, nil, "", &st)
	root.RootNode = rootNode
	return rootNode.(*loopbackNode), nil
}

// ReapChild reaps terminated child processes and returns their PID and exit status.
// It returns true if a child was reaped, false if no child was ready.
func ReapChild() (pid int, status syscall.WaitStatus, err error) {
	// Wait4 with WNOHANG to non-blockingly check for terminated children
	pid, err = syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
	if err != nil {
		return 0, 0, err
	}
	if pid == 0 {
		// No child was ready
		return 0, 0, nil
	}
	return pid, status, nil
}
