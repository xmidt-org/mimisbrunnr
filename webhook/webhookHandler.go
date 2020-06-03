package webhook

import (
	"github.com/SermoDigital/jose/jwt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/provider"
	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/spf13/viper"
	"github.com/xmidt-org/webpa-common/secure"
	"github.com/xmidt-org/webpa-common/secure/handler"
	"github.com/xmidt-org/webpa-common/secure/key"
	"github.com/xmidt-org/webpa-common/xmetrics"
)

const (
	baseURI = "api"
	version = "v3"
)

type JWTValidator struct {
	// JWTKeys is used to create the key.Resolver for JWT verification keys
	Keys key.ResolverFactory

	// Custom is an optional configuration section that defines
	// custom rules for validation over and above the standard RFC rules.
	Custom secure.JWTValidatorFactory
}

type App struct {
	DeviceID    string
	Destination string
	AuthKey     string
}

const (
	MimmisbrunnrMetric = "mimmisbrunnr"
)

const (
	ListSize = "webhook_list_size"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name:      MimmisbrunnrMetric,
			Help:      "Mimmisbrunnr Metric",
			Type:      "counter",
			Namespace: "xmidt",
			Subsystem: "mimmisbrunnr",
		},
	}
}

type Measures struct {
	WebhookListSize metrics.Gauge
}

func NewMeasures(p provider.Provider) *Measures {
	return &Measures{
		WebhookListSize: p.NewGauge(ListSize),
	}
}

func NewPrimaryHandler(l log.Logger, v *viper.Viper, reg *Registry) (*mux.Router, error) {
	var (
		router = mux.NewRouter()
	)

	validator, err := getValidator(v)
	if err != nil {
		return nil, err
	}

	authHandler := handler.AuthorizationHandler{
		HeaderName:          "Authorization",
		ForbiddenStatusCode: 403,
		Validator:           validator,
		Logger:              l,
	}

	authorizationDecorator := alice.New(authHandler.Decorate)

	return configServerRouter(router, authorizationDecorator, reg), nil
}

func configServerRouter(router *mux.Router, primaryHandler alice.Chain, webhookRegistry *Registry) *mux.Router {
	// register webhook end points
	router.Handle("/hook", primaryHandler.ThenFunc(webhookRegistry.UpdateRegistry)).Methods("POST")

	return router
}

func getValidator(v *viper.Viper) (validator secure.Validator, err error) {
	var jwtVals []JWTValidator

	v.UnmarshalKey("jwtValidators", &jwtVals)

	// if a JWTKeys section was supplied, configure a JWS validator
	// and append it to the chain of validators
	validators := make(secure.Validators, 0, len(jwtVals))

	for _, validatorDescriptor := range jwtVals {
		var keyResolver key.Resolver
		keyResolver, err = validatorDescriptor.Keys.NewResolver()
		if err != nil {
			validator = validators
			return
		}

		validators = append(
			validators,
			secure.JWSValidator{
				// DefaultKeyId:  DEFAULT_KEY_ID,
				Resolver:      keyResolver,
				JWTValidators: []*jwt.Validator{validatorDescriptor.Custom.New()},
			},
		)
	}

	// TODO: This should really be part of the unmarshalled validators somehow
	basicAuth := v.GetStringSlice("authHeader")
	for _, authValue := range basicAuth {
		validators = append(
			validators,
			secure.ExactMatchValidator(authValue),
		)
	}

	validator = validators

	return
}
