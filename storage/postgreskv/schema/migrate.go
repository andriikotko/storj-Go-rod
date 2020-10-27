// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

//go:generate go-bindata -o data.go -pkg schema -ignore ".*go" .
//go:generate bash -c "sed -i'' '1i //lint:file-ignore * generated file\n' data.go"

package schema

import (
	"context"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	bindata "github.com/golang-migrate/migrate/v4/source/go_bindata"
	"github.com/zeebo/errs"

	"storj.io/storj/private/dbutil/pgutil"
	"storj.io/storj/private/tagsql"
)

// PrepareDB applies schema migrations as necessary to the given database to
// get it up to date.
func PrepareDB(ctx context.Context, db tagsql.DB, dbURL string) error {
	srcDriver, err := bindata.WithInstance(bindata.Resource(AssetNames(), Asset))
	if err != nil {
		return err
	}

	schema, err := pgutil.ParseSchemaFromConnstr(dbURL)
	if err != nil {
		return errs.New("error parsing schema: %+v", err)
	}
	if schema != "" {
		err := pgutil.CreateSchema(ctx, db, schema)
		if err != nil {
			return errs.New("error creating schema: %+v", err)
		}
	}

	dbDriver, err := postgres.WithInstance(db.Internal(), &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("go-bindata migrations", srcDriver, "postgreskv db", dbDriver)
	if err != nil {
		return err
	}
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		err = nil
	}
	return err
}
