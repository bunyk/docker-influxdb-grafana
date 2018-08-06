package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"syscall"

	"github.com/influxdata/kapacitor/udf/agent"
)

// tradeHandler defines points where to sell and where to buy
type tradeHandler struct {
	// We need a reference to the agent so we can write data
	// back to Kapacitor.
	agent        *agent.Agent
	Capital      float64 // How much money do we have
	Goods        float64 // How much goods did we bought
	LastBuyPrice float64
}

func newTradeHandler(agent *agent.Agent) *tradeHandler {
	return &tradeHandler{agent: agent}
}

func (*tradeHandler) Info() (*agent.InfoResponse, error) {
	info := &agent.InfoResponse{
		// We want a stream edge
		Wants: agent.EdgeType_STREAM,
		// We provide a stream edge
		Provides: agent.EdgeType_STREAM,
		// We expect no options.
		Options: map[string]*agent.OptionInfo{},
	}
	return info, nil
}

// Initialize the handler based of the provided options.
func (th *tradeHandler) Init(r *agent.InitRequest) (*agent.InitResponse, error) {
	// Since we expected no options this method is trivial
	// and we return success.
	// TODO: set initial variables values
	init := &agent.InitResponse{
		Success: true,
		Error:   "",
	}
	th.Capital = 100 // we start from 100% capital
	th.LastBuyPrice = math.MaxFloat64
	return init, nil
}

const buyTreshold = 0.9995
const sellTreshold = 1.005 // sell if profit is more than 0.5%

func (h *tradeHandler) Point(p *agent.Point) error {
	// Send back the point we just received
	if p.FieldsDouble == nil {
		return nil
	}
	if p.Tags == nil {
		p.Tags = make(map[string]string)
	}
	currentPrice := p.FieldsDouble["data_quotes_UAH_price"]
	if h.Capital > 0 && currentPrice < h.LastBuyPrice*buyTreshold { // If we could buy cheap - buy for half of capital
		p.Tags["trade"] = "buy"
		h.LastBuyPrice = currentPrice // Record the price we bought
		h.Goods += h.Capital / currentPrice * 0.5
		h.Capital = h.Capital / 2.0
		fmt.Printf("Price is %f, buying %f goods\n", currentPrice, h.Goods)
	} else if h.Goods > 0 && currentPrice > h.LastBuyPrice*sellTreshold { // Sell if profit is more than treshold
		p.Tags["trade"] = "sell"
		h.Capital += h.Goods * currentPrice
		h.Goods = 0
		h.LastBuyPrice = currentPrice // Record the price we sold, so we do not need to wait for much lowering
		fmt.Printf("Price is %f, selling goods. Now our capital is %f\n", currentPrice, h.Capital)
	}
	p.FieldsDouble["my_capital"] = h.Capital
	p.FieldsDouble["my_goods"] = h.Goods
	h.agent.Responses <- &agent.Response{
		Message: &agent.Response_Point{
			Point: p,
		},
	}
	return nil
}

// Stop the handler gracefully.
func (h *tradeHandler) Stop() {
	// Close the channel since we won't be sending any more data to Kapacitor
	close(h.agent.Responses)
}

// Create a snapshot of the running state of the process.
func (*tradeHandler) Snapshot() (*agent.SnapshotResponse, error) {
	return &agent.SnapshotResponse{}, nil
}

// Restore a previous snapshot.
func (*tradeHandler) Restore(req *agent.RestoreRequest) (*agent.RestoreResponse, error) {
	return &agent.RestoreResponse{
		Success: true,
	}, nil
}

// Start working with the next batch
func (*tradeHandler) BeginBatch(begin *agent.BeginBatch) error {
	return errors.New("batching not supported")
}
func (*tradeHandler) EndBatch(end *agent.EndBatch) error {
	return nil
}

type accepter struct {
	count int64
}

// Create a new agent/handler for each new connection.
// Count and log each new connection and termination.
func (acc *accepter) Accept(conn net.Conn) {
	count := acc.count
	acc.count++
	a := agent.New(conn, conn)
	h := newTradeHandler(a)
	a.Handler = h

	log.Println("Starting agent for connection", count)
	a.Start()
	go func() {
		err := a.Wait()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Agent for connection %d finished", count)
	}()
}

var socketPath = flag.String("socket", "/tmp/trader.sock", "Where to create the unix socket")

func main() {
	flag.Parse()

	// Create unix socket
	addr, err := net.ResolveUnixAddr("unix", *socketPath)
	if err != nil {
		log.Fatal(err)
	}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		log.Fatal(err)
	}

	// Create server that listens on the socket
	s := agent.NewServer(l, &accepter{})

	// Setup signal handler to stop Server on various signals
	s.StopOnSignals(os.Interrupt, syscall.SIGTERM)

	log.Println("Server listening on", addr.String())
	err = s.Serve()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Server stopped")
}
