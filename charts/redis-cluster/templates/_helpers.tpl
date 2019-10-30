{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "chart.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "redis-cluster.fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "helm-toolkit.utils.template" -}}
{{- $name := index . 0 -}}
{{- $context := index . 1 -}}
{{- $last := base $context.Template.Name }}
{{- $wtf := $context.Template.Name | replace $last $name -}}
{{ include $wtf $context }}
{{- end -}}

{{/*
Encapsulate server configmap data for consistent digest calculation
*/}}
{{- define "server-configmap.data" -}}
startup-script: |-
{{ tuple "scripts/_start_server.sh.tpl" . | include "helm-toolkit.utils.template" | indent 2 }}
config-file: |-
    {{- if .Values.redis.config }}
{{ .Values.redis.config | indent 2 }}
    {{- end -}}
{{- if ((.Values.redis.mode) and (eq .Values.redis.mode "replica")) }}
sentinel-config-file: |-
    {{- if .Values.sentinel.config }}
{{ .Values.sentinel.config | indent 4 }}
    {{- else }}
{{ tuple "config/_redis-sentinel-config.tpl" . | include "helm-toolkit.utils.template" | indent 4 }}
    {{- end -}}
{{- end -}}