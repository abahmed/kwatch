<p align="left">
	  <h1>kwatch</h1>
    <br />
    <a href="https://godoc.org/github.com/abahmed/kwatch">
      <img src="https://godoc.org/github.com/abahmed/kwatch?status.png" />
    </a>
    <a href="https://github.com/abahmed/kwatch/actions/workflows/check.yaml">
      <img src="https://github.com/abahmed/kwatch/workflows/Check/badge.svg?branch=main" />
    </a>
    <a href="https://goreportcard.com/report/github.com/abahmed/kwatch">
      <img src="https://goreportcard.com/badge/github.com/abahmed/kwatch" />
    </a>
	<a href="https://discord.gg/kzJszdKmJ7">
      <img src="https://img.shields.io/discord/911647396918870036?label=Discord&logo=discord">
  	</a>
</p>

**kwatch** helps you monitor all changes in your Kubernetes(K8s) cluster, detects crashes in your running apps in realtime, and publishes notifications to your channels (Slack, Discord, etc.)

## Contribute & Support
+ Add a [GitHub Star](https://github.com/abahmed/kwatch/stargazers)
+ [Suggest new features, ideas and optimizations](https://github.com/abahmed/kwatch/issues)
+ [Report issues](https://github.com/abahmed/kwatch/issues)

## Screenshots

<p align="center">
	<img src="https://raw.githubusercontent.com/abahmed/kwatch/main/assets/demo.png" width="60%"/>
</p>

## Getting Started

### Install

You need to get config template to add your configs
```shell
curl  -L https://raw.githubusercontent.com/abahmed/kwatch/v0.0.7/deploy/config.yaml -o config.yaml
```

Then edit `config.yaml` file and apply your configuration

```shell
kubectl apply -f config.yaml
```

To deploy **kwatch**, execute following command:

```shell
kubectl apply -f https://raw.githubusercontent.com/abahmed/kwatch/v0.0.7/deploy/deploy.yaml
```

### Configuration

| Parameter                            | Description                                 |
|:-------------------------------------|:------------------------------------------- |
| `maxRecentLogLines`                  | Max tail log lines in messages              |
| `providers.slack.webhook`            | Slack webhook URL                           |
| `providers.slack.title`              | Customized title in slack message           |
| `providers.slack.text`               | Customized text in slack message            |
| `providers.discord.webhook`          | Discord webhook URL                         |
| `providers.discord.title`            | Customized title in discord message         |
| `providers.discord.text`             | Customized text in discord message          |
| `providers.pagerduty.integrationKey` | PagerDuty integration key [more info](https://support.pagerduty.com/docs/services-and-integrations) |


### Cleanup

```shell
kubectl delete -f https://raw.githubusercontent.com/abahmed/kwatch/v0.0.7/deploy/config.yaml
kubectl delete -f https://raw.githubusercontent.com/abahmed/kwatch/v0.0.7/deploy/deploy.yaml
```

## Contributors

<a href="https://github.com/abahmed/kwatch/graphs/contributors">
  <img src="https://contributors-img.firebaseapp.com/image?repo=abahmed/kwatch" />
</a>

## Get in touch!

Feel free to chat with us on [Discord](https://discord.gg/kzJszdKmJ7) if you have questions, or suggestions

## License

kwatch is licensed under [MIT License](LICENSE)
