---
title: Database
description: Database configuration options
sidebar_position: 2
---

# Database

tvarr supports SQLite, PostgreSQL, and MySQL.

## SQLite (Default)

The simplest option, perfect for single-instance deployments:

```bash
TVARR_DATABASE_DRIVER=sqlite
TVARR_DATABASE_DSN=/data/tvarr.db
```

:::note
SQLite only supports a single replica. For high availability, use PostgreSQL or MySQL.
:::

## PostgreSQL

For production deployments:

```bash
TVARR_DATABASE_DRIVER=postgres
TVARR_DATABASE_DSN="host=localhost user=tvarr password=secret dbname=tvarr port=5432 sslmode=disable"
```

Or connection URL format:

```bash
TVARR_DATABASE_DSN="postgres://tvarr:secret@localhost:5432/tvarr?sslmode=disable"
```

## MySQL

```bash
TVARR_DATABASE_DRIVER=mysql
TVARR_DATABASE_DSN="tvarr:secret@tcp(localhost:3306)/tvarr?charset=utf8mb4&parseTime=True"
```

## Connection Pool

Tune the connection pool for your workload:

```bash
TVARR_DATABASE_MAX_OPEN_CONNS=10
TVARR_DATABASE_MAX_IDLE_CONNS=5
TVARR_DATABASE_CONN_MAX_LIFETIME=1h
```

## Migrations

Migrations run automatically on startup. tvarr tracks applied migrations and only runs new ones.

## Backups

### SQLite

The database is a single file. Back it up like any file:

```bash
# While tvarr is running (SQLite handles this safely)
cp /data/tvarr.db /backup/tvarr-$(date +%Y%m%d).db
```

### PostgreSQL/MySQL

Use native backup tools:

```bash
# PostgreSQL
pg_dump tvarr > backup.sql

# MySQL
mysqldump tvarr > backup.sql
```
