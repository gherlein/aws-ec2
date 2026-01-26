.PHONY: build clean status install

INSTALL_DIR ?= $(HOME)/bin

build:
	mkdir -p bin
	go build -o bin/ec2 main.go

clean:
	rm -rf bin
	rm -f *~

status:
	aws cloudformation describe-stack-events --stack-name $(STACK_NAME) --region $(AWS_REGION)

install: build
	@mkdir -p $(INSTALL_DIR)
	@cp bin/ec2 $(INSTALL_DIR)/ec2
	@chmod +x $(INSTALL_DIR)/ec2
	@echo "Installed ec2 to $(INSTALL_DIR)/ec2"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"
