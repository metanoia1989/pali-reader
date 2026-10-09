# Convenience wrappers. The real entry points are deploy/deploy.sh and the two
# Go commands; this file exists so the common ones are typed the same way every
# time.
.PHONY: help build test dev-frontend deploy deploy-backend deploy-frontend fmt

HOST ?= bigubuntu
ROOT := $(shell pwd)

help:
	@echo "make build           交叉编译后端（linux/amd64）与前端产物"
	@echo "make test            跑后端单元测试"
	@echo "make dev-frontend    起 Vite 开发服务器（代理 /api 到 :8090）"
	@echo "make deploy          编译 + 上传 + 重启 + 自检"
	@echo "make deploy-backend  只发后端"
	@echo "make deploy-frontend 只发前端"

build:
	cd backend && $(ROOT)/go.sh build ./...
	cd frontend && npm run build

test:
	cd backend && $(ROOT)/go.sh vet ./...
	cd backend && $(ROOT)/go.sh test ./...

fmt:
	cd backend && $(ROOT)/go.sh fmt ./...

dev-frontend:
	cd frontend && npm run dev

deploy:
	./deploy/deploy.sh all

deploy-backend:
	./deploy/deploy.sh backend

deploy-frontend:
	./deploy/deploy.sh frontend
