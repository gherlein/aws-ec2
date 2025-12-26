.PHONY: build clean status

build:
	go build -o create-ec2 main.go
	go build -o delete-ec2 delete.go

clean:
	rm -f create-ec2 delete-ec2

status:
	aws cloudformation describe-stack-events --stack-name lowest-cost-x86-instance --region $(AWS_REGION)
