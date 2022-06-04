package reconcilercontroller

import (
	"context"
	"os"

	"github.com/yndd/grpchandlers/pkg/healthhandler"
	"github.com/yndd/grpcserver"
	pkgmetav1 "github.com/yndd/ndd-core/apis/pkg/meta/v1"
	pkgv1 "github.com/yndd/ndd-core/apis/pkg/v1"
	"github.com/yndd/ndd-runtime/pkg/logging"
	"github.com/yndd/registrator/registrator"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ReconcilerController interface {
	// start the reconciler controller
	Start() error
	// stop the reconciler controller
	Stop() error
}

type Options struct {
	Logger            logging.Logger
	GrpcServerAddress string
	Registrator       registrator.Registrator
	Scheme            *runtime.Scheme
}

func New(ctx context.Context, config *rest.Config, o *Options) (ReconcilerController, error) {
	log := o.Logger
	log.Debug("new reconciler controller")

	c, err := client.New(config, client.Options{
		Scheme: o.Scheme,
	})
	if err != nil {
		return nil, err
	}

	r := &reconcilerControllerImpl{
		options:           o,
		stopCh:            make(chan struct{}),
		registrator:       o.Registrator,
		grpcServerAddress: o.GrpcServerAddress,
		log:               o.Logger,
		ctx:               ctx,
		client:            c,
	}

	return r, nil
}

type reconcilerControllerImpl struct {
	options *Options
	log     logging.Logger

	client client.Client

	grpcServerAddress string

	registrator registrator.Registrator
	// server
	server *grpcserver.GrpcServer
	stopCh chan struct{}
	//cfn    context.CancelFunc
	ctx context.Context
}

func (r *reconcilerControllerImpl) Stop() error {
	close(r.stopCh)
	return nil
}

func (r *reconcilerControllerImpl) Start() error {
	log := r.log.WithValues()
	log.Debug("starting reconciler controller...", "grpcServerAddress", r.grpcServerAddress)

	ssw := healthhandler.New(&healthhandler.Options{
		Logger: r.log,
	})

	r.server = grpcserver.New(grpcserver.Config{
		Address: r.grpcServerAddress,
		GNMI:    true,
		Health:  true,
	},
		grpcserver.WithLogger(r.log),
		grpcserver.WithWatchHandler(ssw.Watch),
		grpcserver.WithCheckHandler(ssw.Check),
		grpcserver.WithClient(r.client),
	)
	log.Debug("created grpc server...")
	if err := r.server.Start(r.ctx); err != nil {
		return err
	}
	log.Debug("started grpc server...")

	// register the service
	r.registrator.Register(r.ctx, &registrator.Service{
		Name:    os.Getenv("SERVICE_NAME"),
		ID:      os.Getenv("POD_NAME"),
		Port:    pkgmetav1.GnmiServerPort,
		Address: os.Getenv("POD_IP"),
		//Address:      strings.Join([]string{os.Getenv("POD_NAME"), os.Getenv("GRPC_SVC_NAME"), os.Getenv("POD_NAMESPACE"), "svc", "cluster", "local"}, "."),
		Tags:         pkgv1.GetServiceTag(os.Getenv("POD_NAMESPACE"), os.Getenv("POD_NAME")),
		HealthChecks: []registrator.HealthKind{registrator.HealthKindTTL, registrator.HealthKindGRPC},
	})
	return nil
}
