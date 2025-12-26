.PHONY: build clean status

build:
	mkdir -p bin
	go build -o bin/create-ec2 main.go
	go build -o bin/delete-ec2 delete.go

clean:
	rm -rf bin

status:
	aws cloudformation describe-stack-events --stack-name lowest-cost-x86-instance --region $(AWS_REGION)
