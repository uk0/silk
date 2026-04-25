package core

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	//	"os"
	"regexp"
	"runtime"
)

var logOut io.Writer

type stWriter struct {
	o     io.Writer
	buf   bytes.Buffer
	depth int

	// 匹配此正则表达式时将隐藏整条LOG
	regexHide *regexp.Regexp
	// 匹配此正则表达式时输出堆栈信息
	regexST *regexp.Regexp
}

func (w *stWriter) st(s []byte) {

	fmt.Fprintln(w.o)
	w.o.Write(s)
	for j := 0; j < w.depth; j++ {
		pc, file, line, ok := runtime.Caller(j + 4)
		if !ok {
			break
		}
		fn := runtime.FuncForPC(pc)
		fmt.Fprintf(w.o, "%s:%d: %s()\n", file, line, fn.Name())
	}
	fmt.Fprintln(w.o)
}

func (w *stWriter) Write(p []byte) (n int, err error) {
	for i := 0; i < len(p); i++ {
		w.buf.WriteByte(p[i])
		if p[i] == '\n' {
			s := w.buf.Bytes()
			if w.regexHide != nil && w.regexHide.Match(s) {
				w.buf.Reset()
				continue
			}
			if IsDebugOn() && w.regexST.Match(s) {
				w.st(s)
			} else {
				w.o.Write(s)
			}
			w.buf.Reset()
		}
	}
	return len(p), nil
}

type forkWriter struct {
	a io.Writer
	b io.Writer
}

func (w *forkWriter) Write(p []byte) (int, error) {
	n1, err1 := w.a.Write(p)
	n2, err2 := w.b.Write(p)
	if err1 != nil {
		return n1, err1
	}
	if err2 != nil {
		return n2, err2
	}
	return n1, nil
}

func (w *forkWriter) Close() error {
	closeWriter(w.a)
	closeWriter(w.b)
	return nil
}

func closeWriter(w io.Writer) {
	if w == os.Stdout || w == os.Stderr {
		return
	}
	c, ok := w.(io.Closer)
	if ok {
		c.Close()
	}
}

func closeLogOut() {

	closeWriter(logOut)
	logOut = nil

	log.SetOutput(os.Stdout)
}

// 设置log输出目标, 参见 SetLogOutput
func SetLogOutputFile(filename string, forkToStd bool) (err error) {
	of, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return
	}
	return SetLogOutput(of, forkToStd)
}

// 设置log输出目标
// 此函数会添加一个监视器, 对输出的Log进行分析, 在必要的时候自动添加堆栈信息以便调试
// 通常情况下, 底层会自动调用此函数, 应用层一般不用显式调用
// 注: 调用log.SetOutput() 会覆盖此函数的设置
func SetLogOutput(of io.Writer, forkToStd bool) (err error) {
	if forkToStd && of != os.Stdout {
		of = &forkWriter{os.Stdout, of}
	}

	closeLogOut()

	p := new(stWriter)
	p.o = of
	p.depth = 10
	p.regexST = regexp.MustCompile(`(?i)unimpl|unsupport|warn|fail|fatal|panic|error|\(ww\)|trace`)
	if !isDebugOn {
		p.regexHide = regexp.MustCompile(`(?i)debug|dbg:|trace`)
	}

	log.SetOutput(p)
	if isDebugOn {
		log.SetFlags(log.Ltime)
	}
	return
}
