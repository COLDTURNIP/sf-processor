package sysflow

import (
	"bytes"
	"net"
	"os"

	"github.com/actgardner/gogen-avro/v7/compiler"
	"github.com/actgardner/gogen-avro/v7/vm"
	"github.com/sysflow-telemetry/sf-apis/go/plugins"
	"github.com/sysflow-telemetry/sf-apis/go/sfgo"
	"github.ibm.com/sysflow/goutils/logger"
	"github.ibm.com/sysflow/sf-processor/driver/driver"
	"github.ibm.com/sysflow/sf-processor/driver/pipeline"
)

const (
	// BuffSize represents the buffer size of the stream
	BuffSize = 16384
	// OOBuffSize represents the OO buffer size of the stream
	OOBuffSize = 1024
)

// StreamingDriver represents a streaming sysflow datasource
type StreamingDriver struct {
	pipeline *pipeline.Pipeline
}

// NewStreamingDriver creates a new streaming driver object
func NewStreamingDriver() driver.Driver {
	return &StreamingDriver{}
}

// Init initializes the driver
func (f *StreamingDriver) Init(pipeline *pipeline.Pipeline) error {
	f.pipeline = pipeline
	return nil
}

// Run runs the driver
func (f *StreamingDriver) Run(path string, running *bool) error {
	channel := f.pipeline.GetRootChannel()
	sfChannel := channel.(*plugins.SFChannel)

	records := sfChannel.In
	if err := os.RemoveAll(path); err != nil {
		logger.Error.Println("remove error:", err)
		return err
	}

	l, err := net.ListenUnix("unixpacket", &net.UnixAddr{path, "unixpacket"})
	if err != nil {
		logger.Error.Println("listen error:", err)
		return err
	}
	defer l.Close()

	sFlow := sfgo.NewSysFlow()
	deser, err := compiler.CompileSchemaBytes([]byte(sFlow.Schema()), []byte(sFlow.Schema()))
	if err != nil {
		logger.Error.Println("compiler error:", err)
		return err
	}

	for *running {
		buf := make([]byte, BuffSize)
		oobuf := make([]byte, OOBuffSize)
		reader := bytes.NewReader(buf)
		conn, err := l.AcceptUnix()
		if err != nil {
			logger.Error.Println("accept error:", err)
			break
		}
		for {
			sFlow = sfgo.NewSysFlow()
			_, _, flags, _, err := conn.ReadMsgUnix(buf[:], oobuf[:])
			if err != nil {
				logger.Error.Println("read error:", err)
				break
			}
			if flags == 0 {
				reader.Reset(buf)
				err = vm.Eval(reader, deser, sFlow)
				if err != nil {
					logger.Error.Println("deserialize:", err)
				}
				records <- sFlow
			} else {
				logger.Error.Println("Flag error ReadMsgUnix:", flags)
			}
		}
	}
	logger.Trace.Println("Closing main channel")
	close(records)
	f.pipeline.Wait()
	return nil
}