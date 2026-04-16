module github.com/colony-2/colony2/server/api

go 1.26

require (
	github.com/colony-2/colony2/server/cell v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/core v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/git v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/graph v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/openapi v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/ops v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/pgembed v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/project v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/recipe-child v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/recipe-core v0.0.0-20260403003617-3a350d3426ef
	github.com/colony-2/colony2/server/recipe-input v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/recipe-template v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/recipe-worker v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/recipes v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/registry v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/ticket v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/colony2/server/workflow v0.0.0-20260319012848-6225f21fa882
	github.com/colony-2/pgwf-go v0.0.0-20260324185252-7a5ae14f742a
	github.com/colony-2/strata-go v0.0.0-20260120043346-c4b58c0cd966
	github.com/colony-2/swf-go v0.0.0-20260413043057-cd8d41023399
	github.com/go-playground/validator/v10 v10.19.0
	github.com/gorilla/mux v1.8.1
	github.com/oapi-codegen/runtime v1.1.2
	github.com/spf13/cobra v1.10.1
	github.com/stretchr/testify v1.11.1
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/sqlite v1.5.7
	gorm.io/gorm v1.30.0
	gorm.io/plugin/optimisticlock v1.1.3
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/ProtonMail/go-crypto v1.1.6 // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/cyphar/filepath-securejoin v0.5.1 // indirect
	github.com/docker/cli v29.3.0+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.2 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-git/go-billy/v5 v5.6.2 // indirect
	github.com/go-git/go-git/v5 v5.16.5 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/google/go-github/v84 v84.0.0 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/joho/godotenv v1.5.1 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.17 // indirect
	github.com/mattn/go-shellwords v1.0.12 // indirect
	github.com/moby/go-archive v0.1.0 // indirect
	github.com/moby/moby/api v1.54.0 // indirect
	github.com/moby/moby/client v0.3.0 // indirect
	github.com/moby/patternmatcher v0.6.0 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/nektos/act v0.2.86 // indirect
	github.com/opencontainers/selinux v1.13.1 // indirect
	github.com/pjbgf/sha1cd v0.3.2 // indirect
	github.com/rhysd/actionlint v1.7.11 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/skeema/knownhosts v1.3.1 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	go.yaml.in/yaml/v4 v4.0.0-rc.3 // indirect
	golang.org/x/term v0.38.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	tags.cncf.io/container-device-interface v1.1.0 // indirect
)

require (
	cel.dev/expr v0.24.0 // indirect
	cloud.google.com/go v0.118.3 // indirect
	cloud.google.com/go/auth v0.15.0 // indirect
	cloud.google.com/go/compute/metadata v0.7.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/DataDog/zstd v1.5.7 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/RaduBerinde/axisds v0.1.0 // indirect
	github.com/RaduBerinde/btreemap v0.0.0-20250419174037-3d62b7205d54 // indirect
	github.com/anthropics/anthropic-sdk-go v1.6.2 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cockroachdb/crlib v0.0.0-20241112164430-1264a2edc35b // indirect
	github.com/cockroachdb/errors v1.11.3 // indirect
	github.com/cockroachdb/logtags v0.0.0-20230118201751-21c54148d20b // indirect
	github.com/cockroachdb/pebble/v2 v2.1.4 // indirect
	github.com/cockroachdb/redact v1.1.5 // indirect
	github.com/cockroachdb/swiss v0.0.0-20251224182025-b0f6560f979b // indirect
	github.com/cockroachdb/tokenbucket v0.0.0-20230807174530-cc333fc44b06 // indirect
	github.com/colony-2/colony2/server/gha v0.0.0
	github.com/colony-2/colony2/server/llm v0.0.0-20260319012848-6225f21fa882 // indirect
	github.com/colony-2/colony2/server/runtime v0.0.0
	github.com/colony-2/pgwf v0.0.0-20260324185146-00a58a45f7d8 // indirect
	github.com/colony-2/shai v0.0.8 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v28.3.0+incompatible // indirect
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fergusstrange/embedded-postgres v1.32.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/getkin/kin-openapi v0.133.0 // indirect
	github.com/getsentry/sentry-go v0.27.0 // indirect
	github.com/go-chi/chi/v5 v5.2.2 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/goccy/go-yaml v1.18.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/snappy v0.0.5-0.20231225225746-43d5d4cd4e0e // indirect
	github.com/google/cel-go v0.26.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.5 // indirect
	github.com/googleapis/gax-go/v2 v2.14.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/invopop/jsonschema v0.13.0 // indirect
	github.com/itchyny/gojq v0.12.16 // indirect
	github.com/itchyny/timefmt-go v0.1.6 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/minio/minlz v1.0.1-0.20250507153514-87eb42fe8882 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oapi-codegen/nullable v1.1.0 // indirect
	github.com/oasdiff/yaml v0.0.0-20250309154309-f31be36b4037 // indirect
	github.com/oasdiff/yaml3 v0.0.0-20250309153720-d2182401db90 // indirect
	github.com/openai/openai-go v1.11.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.21.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/segmentio/ksuid v1.0.4 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/swaggest/jsonschema-go v0.3.78 // indirect
	github.com/swaggest/refl v1.4.0 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.9-0.20240815153524-6ea36470d1bd // indirect
	github.com/woodsbury/decimal128 v1.3.0 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.62.0 // indirect
	go.opentelemetry.io/otel v1.40.0 // indirect
	go.opentelemetry.io/otel/metric v1.40.0 // indirect
	go.opentelemetry.io/otel/trace v1.40.0 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/exp v0.0.0-20250218142911-aa4b98e5adaa // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	google.golang.org/genai v1.16.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250929231259-57b25ae835d4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250929231259-57b25ae835d4 // indirect
	google.golang.org/grpc v1.75.1 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gorm.io/driver/postgres v1.5.11 // indirect
)

replace github.com/colony-2/colony2/server/gha => ../gha

replace github.com/colony-2/colony2/server/runtime => ../runtime
