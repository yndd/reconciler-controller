package reconcilercontroller

import (
	"context"
	"os"
	"strings"

	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/ndd-target-runtime/pkg/grpcserver"
	"github.com/yndd/registrator/registrator"
	"k8s.io/client-go/rest"
)

type ReconcilerController interface {
	// start the reconciler controller
	Start() error
	// stop the reconciler controller
	Stop() error
}

type Options struct {
	Logger          logging.Logger
	GrpcBindAddress string
	Registrator     registrator.Registrator
}

func New(ctx context.Context, config *rest.Config, o *Options) (ReconcilerController, error) {
	log := o.Logger
	log.Debug("new reconciler controller")

	r := &reconcilerControllerImpl{
		options:     o,
		stopCh:      make(chan struct{}),
		registrator: o.Registrator,
	}

	return r, nil
}

type reconcilerControllerImpl struct {
	options *Options
	log     logging.Logger

	registrator registrator.Registrator
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
		Name:       os.Getenv("SERVICE_NAME"),
		ID:         os.Getenv("POD_NAME"),
		Port:       pkgmetav1.GnmiServerPort,
		Address:    strings.Join([]string{os.Getenv("POD_NAME"), os.Getenv("GRPC_SVC_NAME"), os.Getenv("POD_NAMESPACE"), "svc", "cluster", "local"}, "."),
		Tags:       pkgmetav1.GetServiceTag(os.Getenv("POD_NAMESPACE"), os.Getenv("POD_NAME")),
		HealthKind: registrator.HealthKindGRPC,
	})
	return nil
}
