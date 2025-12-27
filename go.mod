module github.com/jmylchreest/tvarr

go 1.25.4

require (
	github.com/andybalholm/brotli v1.2.0
	github.com/asticode/go-astits v1.14.0
	github.com/bluenviron/gohlslib/v2 v2.2.4
	github.com/bluenviron/mediacommon/v2 v2.5.3
	github.com/danielgtaylor/huma/v2 v2.34.1
	github.com/dsnet/compress v0.0.1
	github.com/go-chi/chi/v5 v5.2.3
	github.com/google/uuid v1.6.0
	github.com/m-mizutani/masq v0.2.0
	github.com/mattn/go-sqlite3 v1.14.32
	github.com/oklog/ulid/v2 v2.1.1
	github.com/robfig/cron/v3 v3.0.1
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/shirou/gopsutil/v4 v4.25.11
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/ulikunitz/xz v0.5.15
	golang.org/x/image v0.34.0
	golang.org/x/net v0.47.0
	golang.org/x/text v0.32.0
	google.golang.org/grpc v1.77.0
	google.golang.org/protobuf v1.36.10
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/mysql v1.6.0
	gorm.io/driver/postgres v1.6.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/abema/go-mp4 v1.4.1 // indirect
	github.com/asticode/go-astikit v0.57.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-sql-driver/mysql v1.9.3 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.6 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/shoenig/go-m1cpu v0.1.7 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
)

// Fork with E-AC3 support, PCE parsing for channel_config=0, ADTS all profiles fix,
// and WriteTables() for late-joining MPEG-TS clients
// Branch: tvarr (github.com/jmylchreest/mediacommon)
replace github.com/bluenviron/mediacommon/v2 => github.com/jmylchreest/mediacommon/v2 v2.5.4-0.20251222103348-862e6cd6c2fb
