package internal

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackpal/gateway"
	"net"
	"net/netip"
	"os"
)

func ClientRoutine(doingWrite bool, filename string, options map[string]string, ctx context.Context) {
	var err error

	if doingWrite {
		err = DoWriteAsClient(Cfg.Filename, options, ctx)
	} else {
		options["tsize"] = "0"
		err = DoReadAsClient(Cfg.Filename, options, ctx)
	}

	if doingWrite && err != nil {
		os.Exit(21)
	} else if !doingWrite && err != nil {
		os.Exit(22)
	}

	os.Exit(0)
}

func DoReadAsClient(filename string, options map[string]string, ctx context.Context) error {
	var err error
	var alreadyHoldingMessage bool = false

	bestAddressToListenOn, err := FindEnclosingAddress(Cfg.Address)
	if err != nil {
		return err
	}
	pickRandomPort := "0"
	localAddressString := bestAddressToListenOn + ":" + pickRandomPort

	laddr, err := net.ResolveUDPAddr("udp", localAddressString)
	if err != nil {
		fmt.Printf("Unable to resolve %v to address", Cfg.Address)
		return err
	}
	temporaryConnection, err := net.ListenUDP("udp", laddr)
	if err != nil {
		fmt.Println("Unable to listen on:", laddr.String())
		return err
	}
	defer temporaryConnection.Close()
	session, err := NewTftpSession(ctx, temporaryConnection)
	if err != nil {
		fmt.Printf("Unable to create session for address: %v", Cfg.Address)
		return err
	}

	session.Operation = ReadAsClient
	if err = session.ReadMessage(filename, options); err != nil {
		session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("%v", err))
		Log <- NewErrorEvent(session.DestinationAddr.String(), fmt.Sprintf("Failed to send Read Message: %v", err))
		return err
	}

	raddr, err := session.Receive()
	if err != nil {
		return err
	}

	laddr, err = net.ResolveUDPAddr("udp", temporaryConnection.LocalAddr().String())
	session.Destination.Close()
	session.Destination, err = net.DialUDP("udp", laddr, raddr)
	if err != nil {
		return err
	}
	defer session.Destination.Close()

	switch session.MostRecentMessage.(type) {
	case OptionAcknowledgeMessage:
		err = session.UpdateOptions(session.MostRecentMessage.(OptionAcknowledgeMessage).Options)
		if err != nil {
			session.ErrorMessage(ErrorCodeUndefined, "One or more options contain invalid values")
			return errors.New("One or more options contain invalid values")
		}

		if err = session.AcknowledgeMessage(); err != nil {
			fmt.Println("Failed to send ack to: ", session.Destination.RemoteAddr())
			fmt.Println("Error sending acknowledgement: ", err)
			return errors.New("Unable to send acknowledge message!")
		}
	case DataMessage:
		alreadyHoldingMessage = true
	default:
		session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("%v", err))
		Log <- NewErrorEvent(session.DestinationAddr.String(), fmt.Sprintf("Server provided invalid response when opening connection: %v", err))
		return err
	}
	session.LastValidMessage = session.MostRecentMessage

	if session.File, err = os.Create(session.Filename); err != nil {
		session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("Unable to download to: %v", session.Filename))
		Log <- NewErrorEvent(session.DestinationAddr.String(), fmt.Sprintf("Unable to write download to: %v", session.Filename))
		return err
	}
	defer session.File.Close()

	session.BlockNumber = 1

	err = session.ReceiveDataLoop(alreadyHoldingMessage)
	if err != nil {
		return err
	}

	return nil
}

func DoWriteAsClient(filename string, options map[string]string, ctx context.Context) error {
	var err error

	bestAddressToListenOn, err := FindEnclosingAddress(Cfg.Address)
	if err != nil {
		return err
	}
	pickRandomPort := "0"
	localAddressString := bestAddressToListenOn + ":" + pickRandomPort

	laddr, err := net.ResolveUDPAddr("udp", localAddressString)
	if err != nil {
		fmt.Printf("Unable to resolve %v to address", Cfg.Address)
		return err
	}
	temporaryConnection, err := net.ListenUDP("udp", laddr)
	if err != nil {
		fmt.Println("Unable to listen on:", laddr.String())
		return err
	}
	defer temporaryConnection.Close()
	session, err := NewTftpSession(ctx, temporaryConnection)
	if err != nil {
		fmt.Printf("Unable to create session for address: %v", Cfg.Address)
		return err
	}

	session.Operation = WriteAsClient
	if err = session.WriteMessage(filename, options); err != nil {
		session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("%v", err))
		Log <- NewErrorEvent(session.DestinationAddr.String(), fmt.Sprintf("Failed to send Write Message: %v", err))
		return err
	}

	raddr, err := session.Receive()
	if err != nil {
		return err
	}

	laddr, err = net.ResolveUDPAddr("udp", temporaryConnection.LocalAddr().String())
	session.Destination.Close()
	session.Destination, err = net.DialUDP("udp", laddr, raddr)
	if err != nil {
		return err
	}
	defer session.Destination.Close()

	switch session.MostRecentMessage.(type) {
	case OptionAcknowledgeMessage:
		err = session.UpdateOptions(session.MostRecentMessage.(OptionAcknowledgeMessage).Options)
		if err != nil {
			session.ErrorMessage(ErrorCodeUndefined, "One or more options contain invalid values")
			return errors.New("One or more options contain invalid values")
		}
	case AcknowledgeMessage:
		if session.MostRecentMessage.(AcknowledgeMessage).BlockNumber != 0 {
			session.ErrorMessage(ErrorCodeUndefined, "Server did not properly acknowledge client write request")
		}
	default:
		session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("%v", err))
		Log <- NewErrorEvent(session.DestinationAddr.String(), fmt.Sprintf("Server provided invalid response when opening connection: %v", err))
		return err
	}
	session.LastValidMessage = session.MostRecentMessage

	if session.File, err = os.Open(session.Filename); err != nil {
		session.ErrorMessage(ErrorCodeUndefined, fmt.Sprintf("Unable to download to: %v", session.Filename))
		Log <- NewErrorEvent(session.DestinationAddr.String(), fmt.Sprintf("Unable to write download to: %v", session.Filename))
		return err
	}
	defer session.File.Close()

	session.BlockNumber = 1

	err = session.SendDataLoop(false)

	if err != nil {
		return err
	}
	return nil
}

// Find the first address belonging to a network containing destinationString
// Returns the default gateway if no attached network contains destinationString
func FindEnclosingAddress(destinationString string) (string, error) {
	var err error
	var gatewayIP net.IP
	var destinationAddrPort netip.AddrPort
	var destinationNetIP netip.Addr
	var interfaces []net.Addr
	var addr net.Addr
	var network netip.Prefix

	gatewayIP, err = gateway.DiscoverGateway()
	if err != nil {
		return "", err
	}
	destinationAddrPort, err = netip.ParseAddrPort(destinationString)
	if err != nil {
		return "", err
	}
	destinationNetIP, err = netip.ParseAddr(destinationAddrPort.Addr().String())
	if err != nil {
		return "", err
	}

	interfaces, err = net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr = range interfaces {
		network, err = netip.ParsePrefix(addr.String())
		if err != nil {
			return "", err
		}
		if network.Contains(destinationNetIP) {
			return network.Addr().String(), nil
		}
	}

	return gatewayIP.String(), nil
}
