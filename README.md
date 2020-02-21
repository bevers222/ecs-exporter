# AWS Elastic Container Service (ECS) Exporter for Prometheus

This is a service that scrapes the AWS ECS api for service stats and exports them via HTTP for Prometheus to consume.

## Quick Start

Download the binary or install the exporter using go tools: `go get github.com/bevers222/ecs-exporter/cmd/ecs-exporter`

Run the exporter:
```bash
./ecs-exporter -aws.region "$AWS_REGION"
```

This will require AWS credentials or permissions from a role to run. Use the following IAM policy:

```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "",
            "Effect": "Allow",
            "Action": [
                "ecs:ListServices",
                "ecs:ListContainerInstances",
                "ecs:ListClusters",
                "ecs:DescribeServices",
                "ecs:DescribeContainerInstances",
                "ecs:DescribeClusters"
            ],
            "Resource": "*"
        }
    ]
}
```

## Configuration

This exporter can be configured to use assumed roles to poll the ECS api in multiple AWS accounts (referred in tags as islands). You need to create a configuration file and tell the exporter to use it.

```bash
./ecs-exporter -aws.region "$AWS_REGION" -config "$CONFIG_FILE_PATH"
```

Sample configuration file:
```yaml
roles:
  ac1: "arn:aws:iam::123456780123:role/rolename/rolename"
  ac2: "arn:aws:iam::234567801234:role/rolename/rolename"
  long-name-account: "arn:aws:iam::345678012345:role/rolename/rolename"
```

The exporter will collect all metrics for each account. The role you give your exporter will need the access to assume the supplemental roles in the other accounts (islands).

## Flags

The current flags and their use cases are listed here:

- `web.telemetrypath` (string): The path where metrics will be exposed (default: `/metrics`)
- `web.listenAddress` (string): The address to listen on (default: `:9677`)
- `aws.region` (string) (REQUIRED): The AWS region to get metrics from
- `debug` (bool): Runs the exporter in debug mode to provide additional information (default: `false`)
- `config` (string): Config file path

## Exported Metrics

|  Metrics                         | Details                                            | Labels                               |
|----------------------------------|----------------------------------------------------|--------------------------------------|
| ecs_up                           | Was the last query of ecs successful.              | region, island                       |
| ecs_clusters_total               | The total number of ecs clusters.                  | region, island                       |
| ecs_services_total               | The total number of services.                      | region, island, ecsCluster           |
| ecs_service_desired_tasks_total  | The number of tasks to have running.               | region, island, ecsCluster, service  |
| ecs_service_pending_tasks_total  | The number of tasks that are in the PENDING state. | region, island, ecsCluster, service  |
| ecs_service_running_tasks_total  | The number of tasks that are in the RUNNING state. | region, island, ecsCluster, service  |
| ecs_service_deployments_total    | The number of deployments a service has.           | region, island, ecsCluster, service  |

**Note**: island refers to the account name defined in the config file. ie `<account name>: <role to assume>`. If you do not supply a config option, island will be left blank.

## Docker

You can run this exporter using the bevers222/ecs-exporter Docker image

You will need to either mount your `.aws` folder (`-v ~/.aws:/.aws` and `-e AWS_PROFILE=$PROFILE`) or pass in keys (`-e AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY`) to give the exporter access to AWS.

```bash
docker pull bevers222/ecs-exporter
docker run -d -p 9677:9677 bevers222/ecs-exporter -aws.region="us-east-2"
```

A good idea to bring this to your environments is to use the exporter as your base image in a Dockerfile.

```Dockerfile
FROM bevers222/ecs-exporter

# This will pull in your config file and add it to your docker image
ADD config.yml /bin/config.yml

ENTRYPOINT ["/bin/ecs-exporter", "-aws.region=us-east-2", "-config=/bin/config.yml"]
```

## Why another ECS exporter?

Big thanks to the [slock/ecs-exporter](https://github.com/slok/ecs-exporter) for the inspirartion. I decided to build this exporter because the slock/ecs-exporter is not mainainted anymore and I needed to pull the `deployments` metric from each service.

## Feedback and More!

Please feel free to create issues and suggestions on how to improve this exporter. 

Reach out to me on [twitter](https://twitter.com/brandon_evers) or send me and email if you want to chat.

## Future Plans
This is under iteration. More metrics, features, and tests will be implemented in the future. All releases will be versioned and documented.