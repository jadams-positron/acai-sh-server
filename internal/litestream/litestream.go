// Package litestream wraps the Litestream Go API for streaming SQLite WAL
// changes to S3-compatible storage. Run launches replication; if the config
// env vars are absent Run is a no-op so dev runs don't need S3 credentials.
package litestream

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	ls "github.com/benbjohnson/litestream"
	s3client "github.com/benbjohnson/litestream/s3"
)

// Config holds the parameters needed to connect to an S3-compatible bucket.
type Config struct {
	// DBPath is the filesystem path of the SQLite database file to replicate.
	DBPath string
	// Bucket is the S3 bucket name.
	Bucket string
	// Region is the S3 region (e.g. "us-east-1", "nbg1").
	Region string
	// Endpoint overrides the default AWS S3 endpoint for S3-compatible stores
	// (e.g. Hetzner Object Storage).
	Endpoint string
	// KeyPath is the object-key prefix inside the bucket (e.g. "acai.db").
	KeyPath string
}

// FromEnv builds a Config from LITESTREAM_S3_* environment variables.
// Returns nil when LITESTREAM_S3_BUCKET is unset (dev / local mode).
func FromEnv(dbPath string) *Config {
	bucket := os.Getenv("LITESTREAM_S3_BUCKET")
	if bucket == "" {
		return nil
	}
	return &Config{
		DBPath:   dbPath,
		Bucket:   bucket,
		Region:   os.Getenv("LITESTREAM_S3_REGION"),
		Endpoint: os.Getenv("LITESTREAM_S3_ENDPOINT"),
		KeyPath:  os.Getenv("LITESTREAM_S3_PATH"),
	}
}

// Run starts Litestream replication for cfg.DBPath and blocks until ctx is
// canceled. If cfg is nil it logs a single info message and returns nil
// immediately (dev mode).
//
// AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY are read from the environment by
// the S3 client; they do not need to be passed through Config.
func Run(ctx context.Context, log *slog.Logger, cfg *Config) error {
	if cfg == nil {
		log.Info("litestream: not configured — set LITESTREAM_S3_BUCKET to enable replication")
		return nil
	}

	// Build the S3 replica client.
	client := s3client.NewReplicaClient()
	client.Bucket = cfg.Bucket
	client.Region = cfg.Region
	client.Endpoint = cfg.Endpoint
	client.Path = cfg.KeyPath
	// AccessKeyID / SecretAccessKey are picked up from AWS_* env vars by the
	// AWS SDK; explicit override only when set in env to avoid clearing defaults.
	if v := os.Getenv("AWS_ACCESS_KEY_ID"); v != "" {
		client.AccessKeyID = v
	}
	if v := os.Getenv("AWS_SECRET_ACCESS_KEY"); v != "" {
		client.SecretAccessKey = v
	}

	db := ls.NewDB(cfg.DBPath)
	replica := ls.NewReplicaWithClient(db, client)
	db.Replica = replica

	if err := db.Open(); err != nil {
		return fmt.Errorf("litestream: open db: %w", err)
	}
	defer func() {
		if cerr := db.Close(ctx); cerr != nil {
			log.Warn("litestream: close error", "error", cerr)
		}
	}()

	if err := replica.Start(ctx); err != nil {
		return fmt.Errorf("litestream: start replica: %w", err)
	}

	log.Info("litestream: replication started",
		"bucket", cfg.Bucket,
		"region", cfg.Region,
		"endpoint", cfg.Endpoint,
		"key_path", cfg.KeyPath,
	)

	<-ctx.Done()
	log.Info("litestream: shutting down")
	return nil
}

// RunWithRecover wraps Run in a panic-safe loop with exponential backoff.
// It is intended to be launched in a goroutine alongside the HTTP server;
// the provided ctx must be the same context used for graceful shutdown so
// that replication stops cleanly when the server stops.
//
// When cfg is nil (no LITESTREAM_S3_BUCKET configured), Run returns nil
// immediately. We treat that as "intentionally disabled" and exit instead
// of looping — otherwise the goroutine would log-spam every retry interval.
func RunWithRecover(ctx context.Context, log *slog.Logger, cfg *Config) {
	if cfg == nil {
		log.Info("litestream: not configured — set LITESTREAM_S3_BUCKET to enable replication")
		return
	}
	backoff := time.Second
	for ctx.Err() == nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("litestream: panic recovered", "recovered", r)
				}
			}()
			if err := Run(ctx, log, cfg); err != nil {
				log.Error("litestream: run error", "error", err)
			}
		}()
		if ctx.Err() != nil {
			return
		}
		log.Info("litestream: will retry", "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}

// Status reports the replication position of the first replica associated
// with the database at dbPath. Returns the generation and approximate lag.
// Returns an error when the DB cannot be opened or has no replica configured.
func Status(ctx context.Context, log *slog.Logger, cfg *Config) error {
	if cfg == nil {
		fmt.Println("litestream: not configured (LITESTREAM_S3_BUCKET not set)")
		return nil
	}

	client := s3client.NewReplicaClient()
	client.Bucket = cfg.Bucket
	client.Region = cfg.Region
	client.Endpoint = cfg.Endpoint
	client.Path = cfg.KeyPath
	if v := os.Getenv("AWS_ACCESS_KEY_ID"); v != "" {
		client.AccessKeyID = v
	}
	if v := os.Getenv("AWS_SECRET_ACCESS_KEY"); v != "" {
		client.SecretAccessKey = v
	}

	db := ls.NewDB(cfg.DBPath)
	replica := ls.NewReplicaWithClient(db, client)
	db.Replica = replica

	createdAt, err := replica.CreatedAt(ctx)
	if err != nil {
		return fmt.Errorf("litestream status: %w", err)
	}

	pos := replica.Pos()
	log.Info("litestream: status",
		"bucket", cfg.Bucket,
		"key_path", cfg.KeyPath,
		"generation_created_at", createdAt,
		"pos_tx_id", pos.TXID,
	)
	fmt.Printf("litestream status\n  bucket:    %s\n  key_path:  %s\n  created:   %s\n  tx_id:     %d\n",
		cfg.Bucket, cfg.KeyPath, createdAt.Format(time.RFC3339), pos.TXID)
	return nil
}

// Restore pulls the latest generation from S3 and writes it to outputPath.
func Restore(ctx context.Context, log *slog.Logger, cfg *Config, outputPath string) error {
	if cfg == nil {
		return fmt.Errorf("litestream restore: not configured (LITESTREAM_S3_BUCKET not set)")
	}

	client := s3client.NewReplicaClient()
	client.Bucket = cfg.Bucket
	client.Region = cfg.Region
	client.Endpoint = cfg.Endpoint
	client.Path = cfg.KeyPath
	if v := os.Getenv("AWS_ACCESS_KEY_ID"); v != "" {
		client.AccessKeyID = v
	}
	if v := os.Getenv("AWS_SECRET_ACCESS_KEY"); v != "" {
		client.SecretAccessKey = v
	}

	db := ls.NewDB(cfg.DBPath)
	replica := ls.NewReplicaWithClient(db, client)
	db.Replica = replica

	opt := ls.NewRestoreOptions()
	if outputPath != "" {
		opt.OutputPath = outputPath
	}

	log.Info("litestream: restoring", "bucket", cfg.Bucket, "output", opt.OutputPath)
	if err := replica.RestoreV3(ctx, opt); err != nil {
		return fmt.Errorf("litestream restore: %w", err)
	}
	log.Info("litestream: restore complete", "output", opt.OutputPath)
	return nil
}
