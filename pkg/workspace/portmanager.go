package workspace

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/events"
)

type PortReservation struct {
	WorkspaceId uuid.UUID
	WorkloadId  uuid.UUID
	Host        string
	Port        int
}

type PortManager struct {
	startPort int
	items     []PortReservation
}

func NewPortManager(startPOrt int) *PortManager {
	return &PortManager{
		startPort: startPOrt,
		items:     make([]PortReservation, 0),
	}
}

func (p *PortManager) Handle(_ context.Context, envelope events.Envelope) error {
	switch event := envelope.Event().(type) {
	case *OpencodeWorkloadCreated:
		for _, item := range p.items {
			if item.Port == event.Data.Port && item.Host == event.Data.Host && event.Id != item.WorkloadId {
				return fmt.Errorf("port %d already in use", event.Data.Port)
			}
		}
		p.items = append(p.items, PortReservation{
			WorkloadId:  event.Id,
			WorkspaceId: event.WorkspaceId,
			Host:        event.Data.Host,
			Port:        event.Data.Port,
		})
	}
	return nil
}

func (p *PortManager) Reserve(workloadId uuid.UUID, host string) (int, error) {
	port := p.startPort
	for _, ws := range p.items {
		if ws.Host != host {
			continue
		}
		if ws.Port >= port {
			port++
		}
	}
	for isPortBusy(host, strconv.Itoa(port)) {
		port++
	}
	p.items = append(p.items, PortReservation{
		WorkloadId: workloadId,
		Host:       host,
		Port:       port,
	})
	return port, nil
}

func isPortBusy(host, port string) bool {
	slog.Default().Info("checking port busy", "host", host, "port", port)
	listener, err := net.Listen("tcp", ":"+port)
	defer listener.Close()
	return err != nil
}
