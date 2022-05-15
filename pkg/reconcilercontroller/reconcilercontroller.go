package reconcilercontroller

import (
	"context"
	"os"
	"strings"

	"github.com/pkg/errors"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-runtime/pkg/resource"
	"github.com/yndd/ndd-target-runtime/pkg/grpcserver"
	"github.com/yndd/registrator/registrator"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReconcilerController interface {
	// start the reconciler controller
	Start() error
	// stop the reconciler controller
	Stop() error
}

type Options struct {
	Logger                    logging.Logger
	Scheme                    *runtime.Scheme
	GrpcBindAddress           string
	ServiceDiscovery          pkgmetav1.ServiceDiscoveryType
	ServiceDiscoveryNamespace string
	ControllerConfigName      string
}

func New(ctx context.Context, cfg func() *rest.Config, o *Options) (ReconcilerController, error) {
	log := o.Logger
	log.Debug("new reconciler controller")

	r := &reconcilerControllerImpl{
		options: o,
		stopCh:  make(chan struct{}),
	}
	// get client
	client, err := getClient(o.Scheme)
	if err != nil {
		return nil, err
	}
	// create registrator
	ctx, r.cfn = context.WithCancel(ctx)
	switch o.ServiceDiscovery {
	case pkgmetav1.ServiceDiscoveryTypeConsul:
		log.Debug("serviceDiscoveryNamespace", "serviceDiscoveryNamespace", o.ServiceDiscoveryNamespace)
		var err error
		r.registrator, err = registrator.NewConsulRegistrator(ctx, o.ServiceDiscoveryNamespace, "kind-dc1",
			registrator.WithClient(resource.ClientApplicator{
				Client:     client,
				Applicator: resource.NewAPIPatchingApplicator(client),
			}),
			registrator.WithLogger(o.Logger))

		if err != nil {
			return nil, errors.Wrap(err, "Cannot initialize registrator")
		}
	case pkgmetav1.ServiceDiscoveryTypeK8s:
	default:
		r.registrator = registrator.NewNopRegistrator()
	}

	return r, nil
}

type reconcilerControllerImpl struct {
	options *Options
	log     logging.Logger

	registrator registrator.registrator
	// server
	server grpcserver.GrpcServer
	stopCh chan struct{}
	cfn    context.CancelFunc
	ctx    context.Context
}

func (r *reconcilerControllerImpl) Stop() error {
	close(r.stopCh)
	return nil
}

func (r *reconcilerControllerImpl) Start() error {
	r.log.Debug("starting reconciler controller...")

	// start grpc server
	r.server = grpcserver.New(pkgmetav1.GnmiServerPort,
		grpcserver.WithHealth(true),
		grpcserver.WithLogger(r.log),
	)
	if err := r.server.Start(); err != nil {
		return err
	}
	// register the service
	r.registrator.Register(r.ctx, &registrator.Service{
		Name:       pkgmetav1.GetServiceName(r.options.ControllerConfigName, "reconciler"),
		ID:         os.Getenv("POD_NAME"),
		Port:       pkgmetav1.GnmiServerPort,
		Address:    strings.Join([]string{os.Getenv("POD_NAME"), os.Getenv("GRPC_SVC_NAME"), os.Getenv("POD_NAMESPACE"), "svc", "cluster", "local"}, "."),
		Tags:       []string{},
		HealthKind: registrator.HealthKindGRPC,
	})
	return nil
}

func getClient(scheme *runtime.Scheme) (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{Scheme: scheme})
}
