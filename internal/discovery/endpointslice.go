package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/Coosis/k8s-vllm-router/internal/backend"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type EndpointSliceDiscoverer struct {
	client    kubernetes.Interface
	namespace string
	service   string
	portName  string
	scheme    string
	logger    *slog.Logger
}

type EndpointSliceOptions struct {
	Namespace string
	Service   string
	PortName  string
	Scheme    string
	Logger    *slog.Logger
}

func NewEndpointSliceDiscoverer(opts EndpointSliceOptions) (*EndpointSliceDiscoverer, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("create in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return NewEndpointSliceDiscovererWithClient(client, opts), nil
}

func NewEndpointSliceDiscovererWithClient(
	client kubernetes.Interface,
	opts EndpointSliceOptions,
) *EndpointSliceDiscoverer {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	scheme := opts.Scheme
	if scheme == "" {
		scheme = "http"
	}
	return &EndpointSliceDiscoverer{
		client:    client,
		namespace: opts.Namespace,
		service:   opts.Service,
		portName:  opts.PortName,
		scheme:    scheme,
		logger:    logger,
	}
}

func (d *EndpointSliceDiscoverer) Discover(ctx context.Context) ([]backend.Endpoint, error) {
	if d.namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}
	if d.service == "" {
		return nil, fmt.Errorf("service is required")
	}

	slices, err := d.client.DiscoveryV1().
		EndpointSlices(d.namespace).
		List(ctx, metav1.ListOptions{
			LabelSelector: discoveryv1.LabelServiceName + "=" + d.service,
		})
	if err != nil {
		return nil, fmt.Errorf("list endpointslices: %w", err)
	}

	endpoints := make([]backend.Endpoint, 0)
	for _, slice := range slices.Items {
		port, ok := choosePort(slice.Ports, d.portName)
		if !ok {
			continue
		}
		for _, ep := range slice.Endpoints {
			if !endpointReady(ep.Conditions) {
				continue
			}
			for _, address := range ep.Addresses {
				id := endpointID(ep, address)
				endpoints = append(endpoints, backend.Endpoint{
					ID:  id,
					URL: d.scheme + "://" + net.JoinHostPort(address, fmt.Sprint(port)),
				})
			}
		}
	}
	return endpoints, nil
}

// look for a port with the specified name, or return the first port if no name is specified
func choosePort(ports []discoveryv1.EndpointPort, portName string) (int32, bool) {
	for _, port := range ports {
		if port.Port == nil {
			continue
		}
		if portName == "" {
			return *port.Port, true
		}
		if port.Name != nil && *port.Name == portName {
			return *port.Port, true
		}
	}
	return 0, false
}

func endpointReady(conditions discoveryv1.EndpointConditions) bool {
	if conditions.Terminating != nil && *conditions.Terminating {
		return false
	}
	if conditions.Serving != nil {
		return *conditions.Serving
	}
	if conditions.Ready != nil {
		return *conditions.Ready
	}
	return true
}

func endpointID(ep discoveryv1.Endpoint, address string) string {
	if ep.TargetRef != nil && ep.TargetRef.Name != "" {
		return ep.TargetRef.Name
	}
	return address
}
