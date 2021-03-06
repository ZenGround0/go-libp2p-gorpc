package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"
	crypto "github.com/libp2p/go-libp2p-crypto"
	host "github.com/libp2p/go-libp2p-host"
	peer "github.com/libp2p/go-libp2p-peer"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	swarm "github.com/libp2p/go-libp2p-swarm"
	basic "github.com/libp2p/go-libp2p/p2p/host/basic"
	multiaddr "github.com/multiformats/go-multiaddr"
)

func init() {
	logging.SetLogLevel("rpc", "DEBUG")
}

type Args struct {
	A, B int
}

type Quotient struct {
	Quo, Rem int
}

type Arith int

func (t *Arith) Multiply(args *Args, reply *int) error {
	*reply = args.A * args.B
	return nil
}

// This uses non pointer args
func (t *Arith) Add(args Args, reply *int) error {
	*reply = args.A + args.B
	return nil
}

func (t *Arith) Divide(args *Args, quo *Quotient) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}
	quo.Quo = args.A / args.B
	quo.Rem = args.A % args.B
	return nil
}

func (t *Arith) GimmeError(args *Args, r *int) error {
	*r = 42
	return errors.New("an error")
}

func makeRandomNodes() (h1, h2 host.Host) {
	priv1, pub1, _ := crypto.GenerateKeyPair(crypto.RSA, 2048)
	pid1, _ := peer.IDFromPublicKey(pub1)
	maddr1, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/19998")

	priv2, pub2, _ := crypto.GenerateKeyPair(crypto.RSA, 2048)
	pid2, _ := peer.IDFromPublicKey(pub2)
	maddr2, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/19999")

	ps1 := peerstore.NewPeerstore()
	ps2 := peerstore.NewPeerstore()
	ps1.AddPubKey(pid1, pub1)
	ps1.AddPrivKey(pid1, priv1)
	ps1.AddPubKey(pid2, pub2)
	ps1.AddPrivKey(pid2, priv2)
	ps1.AddAddrs(pid2, []multiaddr.Multiaddr{maddr2}, peerstore.PermanentAddrTTL)

	ps2.AddPubKey(pid1, pub1)
	ps2.AddPrivKey(pid1, priv1)
	ps2.AddPubKey(pid2, pub2)
	ps2.AddPrivKey(pid2, priv2)
	ps2.AddAddrs(pid1, []multiaddr.Multiaddr{maddr1}, peerstore.PermanentAddrTTL)

	ctx := context.Background()
	n1, _ := swarm.NewNetwork(
		ctx,
		[]multiaddr.Multiaddr{maddr1},
		pid1,
		ps1,
		nil)
	n2, _ := swarm.NewNetwork(
		ctx,
		[]multiaddr.Multiaddr{maddr2},
		pid2,
		ps2,
		nil)

	h1 = basic.New(n1)
	h2 = basic.New(n2)
	time.Sleep(time.Second)
	return
}

func TestRegister(t *testing.T) {
	h1, h2 := makeRandomNodes()
	defer h1.Close()
	defer h2.Close()
	s := NewServer(h1, "rpc")
	var arith Arith

	err := s.Register(arith)
	if err == nil {
		t.Error("expected an error")
	}
	err = s.Register(&arith)
	if err != nil {
		t.Error(err)
	}
	// Re-register
	err = s.Register(&arith)
	if err == nil {
		t.Error("expected an error")
	}

}

func TestRemote(t *testing.T) {
	h1, h2 := makeRandomNodes()
	defer h1.Close()
	defer h2.Close()
	s := NewServer(h1, "rpc")
	c := NewClientWithServer(h2, "rpc", s)
	var arith Arith
	s.Register(&arith)

	var r int
	err := c.Call(h1.ID(), "Arith", "Multiply", &Args{2, 3}, &r)
	if err != nil {
		t.Fatal(err)
	}
	if r != 6 {
		t.Error("result is:", r)
	}

	var a int
	err = c.Call(h1.ID(), "Arith", "Add", Args{2, 3}, &a)
	if err != nil {
		t.Fatal(err)
	}
	if a != 5 {
		t.Error("result is:", a)
	}

	var q Quotient
	err = c.Call(h1.ID(), "Arith", "Divide", &Args{20, 6}, &q)
	if err != nil {
		t.Fatal(err)
	}
	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}
}

func TestLocal(t *testing.T) {
	h1, h2 := makeRandomNodes()
	defer h1.Close()
	defer h2.Close()

	s := NewServer(h1, "rpc")
	c := NewClientWithServer(h1, "rpc", s)
	var arith Arith
	s.Register(&arith)

	var r int
	err := c.Call("", "Arith", "Multiply", &Args{2, 3}, &r)
	if err != nil {
		t.Fatal(err)
	}
	if r != 6 {
		t.Error("result is:", r)
	}

	var a int
	err = c.Call("", "Arith", "Add", Args{2, 3}, &a)
	if err != nil {
		t.Fatal(err)
	}
	if a != 5 {
		t.Error("result is:", a)
	}

	var q Quotient
	err = c.Call(h1.ID(), "Arith", "Divide", &Args{20, 6}, &q)
	if err != nil {
		t.Fatal(err)
	}
	if q.Quo != 3 || q.Rem != 2 {
		t.Error("bad division")
	}
}

func TestErrorResponse(t *testing.T) {
	h1, h2 := makeRandomNodes()
	defer h1.Close()
	defer h2.Close()

	s := NewServer(h1, "rpc")
	var arith Arith
	s.Register(&arith)

	var r int
	// test remote
	c := NewClientWithServer(h2, "rpc", s)
	err := c.Call(h1.ID(), "Arith", "GimmeError", &Args{1, 2}, &r)
	if err == nil || err.Error() != "an error" {
		t.Error("expected different error")
	}
	if r != 42 {
		t.Error("response should be set even on error")
	}

	// test local
	c = NewClientWithServer(h1, "rpc", s)
	err = c.Call(h1.ID(), "Arith", "GimmeError", &Args{1, 2}, &r)
	if err == nil || err.Error() != "an error" {
		t.Error("expected different error")
	}
	if r != 42 {
		t.Error("response should be set even on error")
	}
}
