IMAGE_NAME ?= flashcatcloud/yunshu
IMAGE_TAG ?= v2.3.10.27

build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
