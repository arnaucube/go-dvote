package swarm

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/node"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/swarm"

	swarmapi "github.com/ethereum/go-ethereum/swarm/api"
	"github.com/ethereum/go-ethereum/swarm/network"
	"github.com/ethereum/go-ethereum/swarm/pss"
)

const (
	// MaxPeers is the maximum number of p2p peer connections
	MaxPeers = 10
)

// SwarmBootnodes list of bootnodes for the SWARM network
var SwarmBootnodes = []string{
	// EF Swarm Bootnode - AWS - eu-central-1
	"enode://4c113504601930bf2000c29bcd98d1716b6167749f58bad703bae338332fe93cc9d9204f08afb44100dc7bea479205f5d162df579f9a8f76f8b402d339709023@3.122.203.99:30301",
	// EF Swarm Bootnode - AWS - us-west-2
	"enode://89f2ede3371bff1ad9f2088f2012984e280287a4e2b68007c2a6ad994909c51886b4a8e9e2ecc97f9910aca538398e0a5804b0ee80a187fde1ba4f32626322ba@52.35.212.179:30301",
}

func newNode(key *ecdsa.PrivateKey, port int, httpport int, wsport int,
	datadir string, modules ...string) (*node.Node, *node.Config, error) {
	if port == 0 {
		port = 30100
	}
	cfg := &node.DefaultConfig
	if key != nil {
		cfg.P2P.PrivateKey = key
	}
	cfg.P2P.MaxPeers = MaxPeers
	cfg.P2P.ListenAddr = fmt.Sprintf("0.0.0.0:%d", port)
	cfg.P2P.EnableMsgEvents = true
	cfg.P2P.NoDiscovery = false
	cfg.P2P.DiscoveryV5 = true
	cfg.IPCPath = datadir + "/node.ipc"
	cfg.DataDir = datadir
	if httpport > 0 {
		cfg.HTTPHost = node.DefaultHTTPHost
		cfg.HTTPPort = httpport
		cfg.HTTPCors = []string{"*"}
	}
	if wsport > 0 {
		cfg.WSHost = node.DefaultWSHost
		cfg.WSPort = wsport
		cfg.WSOrigins = []string{"*"}
		for i := 0; i < len(modules); i++ {
			cfg.WSModules = append(cfg.WSModules, modules[i])
		}
	}
	stack, err := node.New(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ServiceNode create fail: %v", err)
	}
	return stack, cfg, nil
}

func newSwarm(privkey *ecdsa.PrivateKey, datadir string, port int) (*swarm.Swarm, *swarmapi.Config, node.ServiceConstructor) {
	// create swarm service
	swarmCfg := swarmapi.NewConfig()
	swarmCfg.SyncEnabled = true
	swarmCfg.Port = fmt.Sprintf("%d", port)
	swarmCfg.Path = datadir
	swarmCfg.HiveParams.Discovery = true
	swarmCfg.Discovery = true
	swarmCfg.Pss.MsgTTL = time.Second * 10
	swarmCfg.Pss.CacheTTL = time.Second * 30
	swarmCfg.Pss.AllowRaw = true
	swarmCfg.Init(privkey)
	swarmNode, err := swarm.NewSwarm(swarmCfg, nil)
	if err != nil {
		log.Crit("cannot crate swarm node")
	}
	// register swarm service to the node
	var swarmService node.ServiceConstructor = func(ctx *node.ServiceContext) (node.Service, error) {
		return swarmNode, nil
	}
	return swarmNode, swarmCfg, swarmService
}

type swarmPorts struct {
	WebSockets int
	HTTPRPC    int
	Bzz        int
	P2P        int
}

func NewSwarmPorts() *swarmPorts {
	sp := new(swarmPorts)
	sp.WebSockets = 8544
	sp.HTTPRPC = 8543
	sp.Bzz = 8542
	sp.P2P = 31000
	return sp
}

type pssMsg struct {
	Msg   []byte
	Peer  *p2p.Peer
	Asym  bool
	Keyid string
}

type pssSub struct {
	Unregister func()
	Delivery   (chan pssMsg)
	Address    string
}

type SimplePss struct {
	Node       *node.Node
	NodeConfig *node.Config
	EnodeID    string
	Datadir    string
	Key        *ecdsa.PrivateKey
	Pss        *pss.API
	PssPubKey  string
	PssAddr    pss.PssAddress
	PssTopics  map[string]*pssSub
	Hive       *network.Hive
	Ports      *swarmPorts
	LogLevel   string
}

func (sn *SimplePss) SetLog(level string) error {
	// ensure good log formats for terminal
	// handle verbosity flag
	loglevel, err := log.LvlFromString(level)
	if err != nil {
		return err
	}
	hs := log.StreamHandler(os.Stderr, log.TerminalFormat(true))
	hf := log.LvlFilterHandler(loglevel, hs)
	h := log.CallerFileHandler(hf)
	log.Root().SetHandler(h)
	return nil
}

func (sn *SimplePss) PrintStats() {
	// statistics thread
	go func() {
		for {
			if sn.Node.Server() != nil && sn.Hive != nil {
				addr := fmt.Sprintf("%x", sn.PssAddr)
				var addrs [][]byte
				addrs = append(addrs, []byte(addr))
				peerCount := sn.Node.Server().PeerCount()
				log.Info(fmt.Sprintf("PeerCount:%d NeighDepth:%d", peerCount, sn.Hive.NeighbourhoodDepth()))
			}
			time.Sleep(time.Second * 5)
		}
	}()
}

func (sn *SimplePss) SetDatadir(datadir string) {
	sn.Datadir = datadir
}

func (sn *SimplePss) SetKey(key *ecdsa.PrivateKey) {
	sn.Key = key
}

func (sn *SimplePss) Init() error {
	var err error
	if len(sn.Datadir) < 1 {
		usr, err := user.Current()
		if err != nil {
			return err
		}
		sn.Datadir = usr.HomeDir + "/.dvote/swarm"
		os.MkdirAll(sn.Datadir, 0755)
	}

	sn.SetLog(sn.LogLevel)

	// if sn.Ports are not defined, set the default config
	if (sn.Ports.WebSockets == 0) && (sn.Ports.HTTPRPC == 0) && (sn.Ports.Bzz == 0) && (sn.Ports.P2P == 0) {
		sn.Ports = NewSwarmPorts()
	}

	// create node
	sn.Node, sn.NodeConfig, err = newNode(sn.Key, sn.Ports.P2P,
		sn.Ports.HTTPRPC, sn.Ports.WebSockets, sn.Datadir, "pss")
	if err != nil {
		return err
	}
	// set node key, if not set use the storage one or generate it
	if sn.Key == nil {
		sn.Key = sn.NodeConfig.NodeKey()
	}

	// create and register Swarm service
	swarmNode, _, swarmHandler := newSwarm(sn.Key, sn.Datadir, sn.Ports.Bzz)
	err = sn.Node.Register(swarmHandler)
	if err != nil {
		return fmt.Errorf("swarm register fail %v", err)
	}

	// start the node
	err = sn.Node.Start()
	if err != nil {
		return err
	}
	for _, url := range SwarmBootnodes {
		log.Info("Add bootnode " + url)
		node, err := enode.ParseV4(url)
		if err != nil {
			return err
		}
		sn.Node.Server().AddPeer(node)
	}

	// wait to connect to the p2p network
	_, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	time.Sleep(time.Second * 5)

	// Get the services API
	for _, a := range swarmNode.APIs() {
		switch a.Service.(type) {
		case *network.Hive:
			sn.Hive = a.Service.(*network.Hive)
		case *pss.API:
			sn.Pss = a.Service.(*pss.API)
		}
	}

	// Create topics map
	sn.PssTopics = make(map[string]*pssSub)

	// Set some extra data
	sn.EnodeID = sn.Node.Server().NodeInfo().Enode
	sn.PssPubKey = hexutil.Encode(crypto.FromECDSAPub(sn.Pss.PublicKey()))
	sn.PssAddr, err = sn.Pss.BaseAddr()
	if err != nil {
		return fmt.Errorf("pss API fail %v", err)
	}

	// Print some information
	log.Info(fmt.Sprintf("My PSS pubkey is %s", sn.PssPubKey))
	log.Info(fmt.Sprintf("My PSS address is %x", sn.PssAddr))

	// Run statistics goroutine
	sn.PrintStats()

	return nil
}

func strTopic(topic string) pss.Topic {
	return pss.BytesToTopic([]byte(topic))
}

func strSymKey(key string) []byte {
	symKey := make([]byte, 32)
	copy(symKey, []byte(key))
	return symKey
}

func strAddress(addr string) pss.PssAddress {
	var pssAddress pss.PssAddress
	pssAddress = []byte(addr)
	return pssAddress
}

func (sn *SimplePss) PssSub(subType, key, topic, address string) error {
	pssTopic := strTopic(topic)
	pssAddress := strAddress(address)
	if subType == "sym" {
		_, err := sn.Pss.SetSymmetricKey(strSymKey(key), pssTopic, pssAddress, true)
		if err != nil {
			return err
		}
	}

	sn.PssTopics[topic] = new(pssSub)
	sn.PssTopics[topic].Address = address
	sn.PssTopics[topic].Delivery = make(chan pssMsg)

	var pssHandler pss.HandlerFunc = func(msg []byte, peer *p2p.Peer, asym bool, keyid string) error {
		log.Debug("pss received", "msg", fmt.Sprintf("%s", msg), "keyid", fmt.Sprintf("%s", keyid))

		sn.PssTopics[topic].Delivery <- pssMsg{Msg: msg, Peer: peer, Asym: asym, Keyid: keyid}
		return nil
	}
	topicHandler := pss.NewHandler(pssHandler)
	if subType == "raw" {
		topicHandler = topicHandler.WithProxBin().WithRaw()
	}
	sn.PssTopics[topic].Unregister = sn.Pss.Register(&pssTopic, topicHandler)

	log.Info(fmt.Sprintf("Subscribed to [%s] topic %s", subType, pssTopic.String()))
	return nil
}

func (sn *SimplePss) PssPub(subType, key, topic, msg, address string) error {
	var err error
	dstAddr := strAddress(address)
	dstTopic := strTopic(topic)
	if subType == "sym" {
		symKeyId, err := sn.Pss.SetSymmetricKey(strSymKey(key), dstTopic, dstAddr, false)
		if err != nil {
			return err
		}
		// send symetric message
		err = sn.Pss.SendSym(symKeyId, strTopic(topic), hexutil.Bytes(msg))
	}
	if subType == "raw" {
		// sed raw message
		err = sn.Pss.SendRaw(hexutil.Bytes(address), dstTopic, hexutil.Bytes(msg))
	}
	if subType == "asym" {
		// add 0x prefix if not present
		if hasHexPrefix := strings.HasPrefix(key, "0x"); !hasHexPrefix {
			key = "0x" + key
		}

		// check if topic+address is already set for a pubKey
		_, err := sn.Pss.GetPeerAddress(key, dstTopic)
		if err != nil {
			pubKeyBytes, err := hexutil.Decode(key)
			if err != nil {
				return err
			}
			err = sn.Pss.SetPeerPublicKey(pubKeyBytes, dstTopic, dstAddr)
			if err != nil {
				return err
			}
		}

		// send asymetric message
		err = sn.Pss.SendAsym(key, dstTopic, hexutil.Bytes(msg))
	}
	return err
}
