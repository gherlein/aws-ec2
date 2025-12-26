.PHONY: build clean status

build:
	mkdir -p bin
	go build -o bin/ec2 main.go

clean:
	rm -rf bin

status:
	aws cloudformation describe-stack-events --stack-name $(STACK_NAME) --region $(AWS_REGION)
