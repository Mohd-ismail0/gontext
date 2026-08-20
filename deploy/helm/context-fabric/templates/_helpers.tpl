{{- define "context-fabric.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "context-fabric.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "context-fabric.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" -}}
{{- end -}}

{{- define "context-fabric.labels" -}}
helm.sh/chart: {{ include "context-fabric.chart" . }}
app.kubernetes.io/name: {{ include "context-fabric.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
context-fabric.io/profile: {{ .Values.profile | quote }}
{{- end -}}

{{- define "context-fabric.selectorLabels" -}}
app.kubernetes.io/name: {{ include "context-fabric.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "context-fabric.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "context-fabric.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "context-fabric.image" -}}
{{- if .Values.image.digest -}}
{{ printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else -}}
{{ printf "%s:%s" .Values.image.repository .Values.image.tag }}
{{- end -}}
{{- end -}}

{{- define "context-fabric.env" -}}
- name: CONTEXT_FABRIC_PROFILE
  value: {{ .Values.profile | quote }}
- name: CONTEXT_FABRIC_LISTEN_ADDR
  value: {{ .Values.config.listenAddr | quote }}
- name: CONTEXT_FABRIC_LOG_LEVEL
  value: {{ .Values.config.logLevel | quote }}
- name: CAPABILITY_PGVECTOR
  value: {{ .Values.config.capabilities.pgvector | quote }}
- name: CAPABILITY_JETSTREAM_PERSISTENCE
  value: {{ .Values.config.capabilities.jetstream_persistence | quote }}
- name: CAPABILITY_S3_VERSIONING
  value: {{ .Values.config.capabilities.s3_versioning | quote }}
- name: CAPABILITY_S3_OBJECT_LOCK
  value: {{ .Values.config.capabilities.s3_object_lock | quote }}
- name: CAPABILITY_OPENFGA_STORE
  value: {{ .Values.config.capabilities.openfga_store | quote }}
- name: POSTGRES_HOST
  value: {{ .Values.dependencies.postgres.host | quote }}
- name: POSTGRES_PORT
  value: {{ .Values.dependencies.postgres.port | quote }}
- name: POSTGRES_DB
  value: {{ .Values.dependencies.postgres.database | quote }}
- name: POSTGRES_SSLMODE
  value: {{ .Values.dependencies.postgres.sslMode | quote }}
- name: POSTGRES_USER
  valueFrom:
    secretKeyRef:
      name: {{ .Values.dependencies.postgres.existingSecret }}
      key: {{ .Values.dependencies.postgres.gatewayUserKey }}
- name: POSTGRES_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Values.dependencies.postgres.existingSecret }}
      key: {{ .Values.dependencies.postgres.gatewayPasswordKey }}
- name: S3_ENDPOINT
  value: {{ .Values.dependencies.s3.endpoint | quote }}
- name: S3_REGION
  value: {{ .Values.dependencies.s3.region | quote }}
- name: S3_PATH_STYLE
  value: {{ .Values.dependencies.s3.pathStyle | quote }}
- name: S3_BUCKET_RAW
  value: {{ .Values.dependencies.s3.buckets.raw | quote }}
- name: S3_BUCKET_DERIVED
  value: {{ .Values.dependencies.s3.buckets.derived | quote }}
- name: S3_BUCKET_QUARANTINE
  value: {{ .Values.dependencies.s3.buckets.quarantine | quote }}
- name: S3_ACCESS_KEY_ID
  valueFrom:
    secretKeyRef:
      name: {{ .Values.dependencies.s3.existingSecret }}
      key: {{ .Values.dependencies.s3.accessKeyKey }}
- name: S3_SECRET_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.dependencies.s3.existingSecret }}
      key: {{ .Values.dependencies.s3.secretKeyKey }}
- name: NATS_URL
  value: {{ .Values.dependencies.nats.url | quote }}
- name: NATS_DOMAIN
  value: {{ .Values.dependencies.nats.domain | quote }}
- name: NATS_CREDENTIALS
  valueFrom:
    secretKeyRef:
      name: {{ .Values.dependencies.nats.existingSecret }}
      key: {{ .Values.dependencies.nats.credentialsKey }}
      optional: true
- name: OPENFGA_API_URL
  value: {{ .Values.dependencies.openfga.apiUrl | quote }}
- name: OPENFGA_MODEL_ID
  value: {{ .Values.config.openfgaModelId | quote }}
- name: OPENFGA_API_TOKEN
  valueFrom:
    secretKeyRef:
      name: {{ .Values.dependencies.openfga.existingSecret }}
      key: {{ .Values.dependencies.openfga.apiTokenKey }}
- name: OPENFGA_STORE_ID
  valueFrom:
    secretKeyRef:
      name: {{ .Values.dependencies.openfga.existingSecret }}
      key: {{ .Values.dependencies.openfga.storeIdKey }}
- name: OIDC_ISSUER
  value: {{ .Values.dependencies.oidc.issuer | quote }}
- name: OIDC_AUDIENCE
  value: {{ .Values.dependencies.oidc.audience | quote }}
- name: OIDC_CLIENT_ID
  value: {{ .Values.dependencies.oidc.clientId | quote }}
- name: OIDC_DISCOVERY_URL
  value: {{ .Values.dependencies.oidc.discoveryUrl | quote }}
- name: OIDC_JWKS_URL
  value: {{ .Values.dependencies.oidc.jwksUrl | quote }}
- name: OIDC_CLIENT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ .Values.dependencies.oidc.existingSecret }}
      key: {{ .Values.dependencies.oidc.clientSecretKey }}
- name: CONTEXT_FABRIC_CONFIG
  value: /etc/context-fabric/config.yaml
{{- end -}}
