package pki

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

// Factory creates a new backend implementing the logical.Backend interface
func Factory(ctx context.Context, conf *logical.BackendConfig) (logical.Backend, error) {
	b := Backend(conf)
	if err := b.Setup(ctx, conf); err != nil {
		return nil, err
	}
	return b, nil
}

// Backend returns a new Backend framework struct
func Backend(conf *logical.BackendConfig) *backend {
	var b backend
	b.Backend = &framework.Backend{
		PeriodicFunc: b.periodicFunc,
		Help:         strings.TrimSpace(backendHelp),

		PathsSpecial: &logical.Paths{
			Unauthenticated: []string{
				"cert/*",
				"ca/pem",
				"ca_chain",
				"ca",
				"crl/pem",
				"crl",
			},

			LocalStorage: []string{
				"revoked/",
				"crl",
				"certs/",
			},

			Root: []string{
				"root",
				"root/sign-self-issued",
			},

			SealWrapStorage: []string{
				"config/ca_bundle",
			},
		},

		Paths: []*framework.Path{
			pathListRoles(&b),
			pathRoles(&b),
			pathGenerateRoot(&b),
			pathSignIntermediate(&b),
			pathSignSelfIssued(&b),
			pathDeleteRoot(&b),
			pathGenerateIntermediate(&b),
			pathSetSignedIntermediate(&b),
			pathConfigCA(&b),
			pathConfigCRL(&b),
			pathConfigURLs(&b),
			pathSignVerbatim(&b),
			pathSign(&b),
			pathIssue(&b),
			pathRotateCRL(&b),
			pathFetchCA(&b),
			pathFetchCAChain(&b),
			pathFetchCRL(&b),
			pathFetchCRLViaCertPath(&b),
			pathFetchValid(&b),
			pathFetchListCerts(&b),
			pathRevoke(&b),
			pathTidy(&b),
		},

		Secrets: []*framework.Secret{
			secretCerts(&b),
		},

		BackendType: logical.TypeLogical,
	}

	b.crlLifetime = time.Hour * 72
	b.periodicTidyInterval = time.Hour * 48
	b.tidyCASGuard = new(uint32)
	b.storage = conf.StorageView

	return &b
}

type backend struct {
	*framework.Backend

	storage              logical.Storage
	crlLifetime          time.Duration
	periodicTidyInterval time.Duration
	revokeStorageLock    sync.RWMutex
	tidyCASGuard         *uint32
	lastTidyTime         time.Time
}

// This periodicFunc will be invoked once per minute by the RollbackManager
// This removes stale CRL entries and expired certificates
func (b *backend) periodicFunc(ctx context.Context, req *logical.Request) error {
	bufferDuration := defaultSafetyBufferDuration * time.Second
	if time.Now().Sub(b.lastTidyTime) > b.periodicTidyInterval {
		b.lastTidyTime = time.Now()
		return b.tidyPKI(ctx, req, bufferDuration, true, true, true)
	}
	return nil
}

const backendHelp = `
The PKI backend dynamically generates X509 server and client certificates.

After mounting this backend, configure the CA using the "pem_bundle" endpoint within
the "config/" path.
`
