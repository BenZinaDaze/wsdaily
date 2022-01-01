
NAME = wsrc
os = windows
arch = amd64
CONF_FILE = conf.yaml
ZIP_DIR = releases
VERSION = $(shell git describe --abbrev=0)
DIR := ${NAME}-${os}-${VERSION}
ZIP_NAME := ${NAME}-${os}-${arch}-${VERSION}
ifeq ($(os),windows) 
	NAME := ${NAME}.exe
endif
%:
    @:
		@echo "请输入参数"
run:
	@go run ./
build:
	@CGO_ENABLED=0 GOOS=${os} GOARCH=${arch} go build -o ${NAME}
zip:
	@CGO_ENABLED=0 GOOS=${os} GOARCH=${arch} go build -o ${NAME}
	@echo ${os}-${arch} 编译成功
	@if [ ! -e $(DIR) ]; then mkdir ${DIR}; fi
	@if [ ! -e $(ZIP_DIR) ]; then mkdir ${ZIP_DIR}; fi
	@echo "cron: 0 30 6,14,22 * * *\nlogins:\n    - login: xxxxx\n      password: xxxxx\n      server: 1\n    - login: yyyyy\n      password: yyyyy\n      server: 2" > ./${DIR}/${CONF_FILE}
	@mv ${NAME} ${DIR}
	@zip -q -r ./${ZIP_DIR}/${ZIP_NAME}.zip ${DIR}
	@echo ${os}-${arch} 压缩成功
	@rm ${DIR} -rf
all:
	@$(MAKE) zip os=windows arch=amd64 --no-print-directory
	@$(MAKE) zip os=linux arch=amd64 --no-print-directory
	@$(MAKE) zip os=linux arch=arm --no-print-directory
	@$(MAKE) zip os=darwin arch=amd64 --no-print-directory
	