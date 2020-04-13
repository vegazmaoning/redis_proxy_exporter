GO ?= go

all: build

release: format build

build:
	@echo $(GO)
	$(GO) build  -o predixy_exporter .

format:
	$(GO) fmt -w *.go

clean:
	@rm -rf predixy_exporter
	@echo Done.
