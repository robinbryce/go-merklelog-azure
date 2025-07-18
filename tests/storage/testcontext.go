package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/datatrails/go-datatrails-common/azblob"
	commoncbor "github.com/datatrails/go-datatrails-common/cbor"
	"github.com/datatrails/go-datatrails-common/logger"
	"github.com/datatrails/go-datatrails-merklelog/massifs"
	"github.com/datatrails/go-datatrails-merklelog/massifs/storage"
	"github.com/robinbryce/go-merklelog-azure/datatrails"
	azstorage "github.com/robinbryce/go-merklelog-azure/storage"
	"github.com/robinbryce/go-merklelog-provider-testing/mmrtesting"
	"github.com/stretchr/testify/require"
)

type TestContext struct {
	mmrtesting.TestGenerator
	Cfg    *TestOptions
	Log    logger.Logger
	Storer *azblob.Storer
}

type TestOptions struct {
	mmrtesting.TestOptions
	Container  string // can be "" defaults to TestLablePrefix
	DebugLevel string // defaults to INFO
}

func WithContainer(container string) mmrtesting.Option {
	return func(o any) {
		options, ok := o.(*TestOptions)
		if !ok {
			return
		}
		options.Container = container
	}
}

func NewDefaultTestContext(t *testing.T, opts ...mmrtesting.Option) *TestContext {
	opts = append([]mmrtesting.Option{mmrtesting.WithDefaults()}, opts...)
	return NewTestContext(t, nil, nil, opts...)
}

// Satisfy the provider tests interface

func (c *TestContext) GetTestCfg() mmrtesting.TestOptions {
	return c.Cfg.TestOptions
}

func (c *TestContext) GetT() *testing.T {
	return c.TestGenerator.T
}

func (c *TestContext) NewMassifCommitter(opts storage.Options) (storage.MassifCommitter, error) {
	return c.NewNativeMassifCommitter(opts)
}

func (c *TestContext) NewMassifContextReader(opts storage.Options) (storage.MassifContextReader, error) {
	return c.NewNativeMassifCommitter(opts)
}

// end interface implementation

func NewTestContext(t *testing.T, c *TestContext, cfg *TestOptions, opts ...mmrtesting.Option) *TestContext {

	if cfg == nil {
		cfg = &TestOptions{}
	}
	for _, opt := range opts {
		opt(&cfg.TestOptions)
		opt(cfg)
	}

	logLevel := cfg.DebugLevel
	if logLevel == "" {
		logLevel = "NOOP"
		cfg.DebugLevel = logLevel
	}
	logger.New(logLevel)

	if c == nil {
		c = &TestContext{
			Cfg: cfg,
		}
	}
	c.TestGenerator.Init(t, &cfg.TestOptions)

	c.Log = logger.Sugar.WithServiceName(cfg.TestOptions.TestLabelPrefix)
	if c.Cfg.Container == "" {
		cfg.Container = strings.ReplaceAll(strings.ToLower(cfg.TestOptions.TestLabelPrefix), "_", "")
	}

	var err error
	c.Storer, err = azblob.NewDev(azblob.NewDevConfigFromEnv(), cfg.Container)
	if err != nil {
		t.Fatalf("failed to connect to blob store emulator: %v", err)
	}
	client := c.Storer.GetServiceClient()
	// Note: we expect a 'already exists' error here and  ignore it.
	_, _ = client.CreateContainer(t.Context(), cfg.Container, nil)

	return c
}

func (c *TestContext) NewNativeMassifCommitter(opts storage.Options) (*azstorage.MassifCommitter, error) {
	var err error
	if opts.CBORCodec == nil {
		var codec commoncbor.CBORCodec
		codec, err = massifs.NewCBORCodec()
		if err != nil {
			return nil, err
		}
		opts.CBORCodec = &codec
	}
	if opts.PathProvider == nil {
		opts.PathProvider = datatrails.NewFixedPaths(opts.LogID)
	}
	azopts := azstorage.Options{
		Options: opts,
		Store:   c.Storer,
	}
	return azstorage.NewMassifCommitter(azopts)
}

func (c *TestContext) GetLog() logger.Logger { return c.Log }

func (c *TestContext) GetStorer() *azblob.Storer {
	return c.Storer
}

func (c *TestContext) NewStorer() *azblob.Storer {
	storer, err := azblob.NewDev(azblob.NewDevConfigFromEnv(), c.Cfg.Container)
	if err != nil {
		c.T.Fatalf("failed to connect to blob store emulator: %v", err)
	}
	client := storer.GetServiceClient()
	// Note: we expect a 'already exists' error here and  ignore it.
	_, _ = client.CreateContainer(context.Background(), c.Cfg.Container, nil)

	return storer
}

func (c *TestContext) DeleteBlobsByPrefix(blobPrefixPath string) {
	var err error
	var r *azblob.ListerResponse
	var blobs []string

	var marker azblob.ListMarker
	for {
		r, err = c.Storer.List(
			context.Background(),
			azblob.WithListPrefix(blobPrefixPath), azblob.WithListMarker(marker) /*, azblob.WithListTags()*/)

		require.NoError(c.T, err)

		for _, i := range r.Items {
			blobs = append(blobs, *i.Name)
		}
		if len(r.Items) == 0 {
			break
		}

		// check for an empty marker as well as a nil marker
		if r.Marker == nil || *r.Marker == "" {
			break
		}
		marker = r.Marker
	}
	for _, blobPath := range blobs {
		err = c.Storer.Delete(context.Background(), blobPath)
		require.NoError(c.T, err)
	}
}
