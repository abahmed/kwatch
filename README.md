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
    <a href="https://codecov.io/gh/abahmed/kwatch">
      <img src="https://codecov.io/gh/abahmed/kwatch/branch/main/graph/badge.svg?token=ZMCU75JJO7"/>
    </a>
    <a href="https://github.com/abahmed/kwatch/releases/latest">
      <img src="https://img.shields.io/github/v/release/abahmed/kwatch?label=kwatch" />
    </a>
	  <a href="https://discord.gg/kzJszdKmJ7">
      <img src="https://img.shields.io/discord/911647396918870036?label=Discord&logo=discord">
  	</a>
</p>

**kwatch** helps you monitor all changes in your Kubernetes(K8s) cluster, detects crashes in your running apps in realtime, and publishes notifications to your channels (Slack, Discord, etc.) instantly

## Contribute & Support
+ Add a [GitHub Star](https://github.com/abahmed/kwatch/stargazers)
+ [Suggest new features, ideas and optimizations](https://github.com/abahmed/kwatch/issues)
+ [Report issues](https://github.com/abahmed/kwatch/issues)


## Getting Started

### Install

You need to get config template to add your configs
```shell
curl  -L https://raw.githubusercontent.com/abahmed/kwatch/v0.2.0/deploy/config.yaml -o config.yaml
```

Then edit `config.yaml` file and apply your configuration

```shell
kubectl apply -f config.yaml
```

To deploy **kwatch**, execute following command:

```shell
kubectl apply -f https://raw.githubusercontent.com/abahmed/kwatch/v0.2.0/deploy/deploy.yaml
```

### Configuration

#### General

| Parameter                            | Description                                                                                          |
|:-------------------------------------|:-----------------------------------------------------------------------------------------------------|
| `maxRecentLogLines`                  | Optional Max tail log lines in messages, if it's not provided it will get all log lines              |
| `namespaces`                         | Optional list of namespaces that you want to watch, if it's not provided it will watch all namespaces|


#### Slack

<p>
	<img src="./assets/slack.png" width="30%"/>
</p>

If you want to enable Slack, provide the webhook with optional text and title


| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.slack.webhook`            | Slack webhook URL                           |
| `alert.slack.title`              | Customized title in slack message           |
| `alert.slack.text`               | Customized text in slack message            |

#### Discord

<p>
	<img src="./assets/discord.png" width="30%"/>
</p>

If you want to enable Discord, provide the webhook with optional text and title

| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.discord.webhook`          | Discord webhook URL                         |
| `alert.discord.title`            | Customized title in discord message         |
| `alert.discord.text`             | Customized text in discord message          |

#### Email

<p>
    <img src="./assets/email.png" width="50%"/>
</p>

If you want to enable Email, provide the from and to emails with host and the port

| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.email.from`               | From email                                  |
| `alert.email.password`           | From email Password                         |
| `alert.email.host`               | provide the host                            |
| `alert.email.port`               | provide the port                            |
| `alert.email.to`                 | the receiver email                          |


#### PagerDuty

<p>
	<img src="./assets/pagerduty.png" width="50%"/>
</p>

If you want to enable PagerDuty, provide the integration key

| Parameter                        | Description                                 |
|:---------------------------------|:------------------------------------------- |
| `alert.pagerduty.integrationKey` | PagerDuty integration key [more info](https://support.pagerduty.com/docs/services-and-integrations) |

#### Telegram

<p>
    <img src="./assets/telegram.png" width="50%"/>
</p>

If you want to enable Telegram, provide a valid token and the chat Id.

| Parameter                        | Description                                     |
|:---------------------------------|:------------------------------------------------|
| `alert.telegram.token`           | Telegram token                                  |
| `alert.telegram.chatId`          | Telegram chat id                                |

#### Microsoft Teams

<p>
    <img src="./assets/teams.png" width="50%"/>
</p>

If you want to enable Microsoft Teams, provide the channel webhook.

| Parameter                        | Description                                     |
|:---------------------------------|:------------------------------------------------|
| `alert.teams.webhook`            |  webhook Microsoft team                         |
| `alert.teams.title`              | Customized title in Microsoft teams message     |
| `alert.teams.text`              | Customized title in Microsoft teams message     |

#### Rocket Chat

<p>
	<img src="./assets/rocketchat.png" width="50%"/>
</p>

If you want to enable Rocket Chat, provide the webhook with optional text

| Parameter                  | Description                            |
|:---------------------------|:---------------------------------------|
| `alert.rocketchat.webhook` | Rocket Chat webhook URL                |
| `alert.rocketchat.text`    | Customized text in rocket chat message |

### Cleanup

```shell
kubectl delete -f https://raw.githubusercontent.com/abahmed/kwatch/v0.2.0/deploy/config.yaml
kubectl delete -f https://raw.githubusercontent.com/abahmed/kwatch/v0.2.0/deploy/deploy.yaml
```

## Who uses kwatch?

**kwatch** is being used by multiple entities including, but not limited to

[<img src="./assets/users/trella.png"/>](https://www.trella.app)
[<img src="./assets/users/ibec-systems.svg" width="50%"/>](https://ibecsystems.com/en#/)

If you want to add your entity, [open issue](https://github.com/abahmed/kwatch/issues) to add it

## Contributors

<a href="https://github.com/abahmed/kwatch/graphs/contributors">
  <img src="https://contributors-img.firebaseapp.com/image?repo=abahmed/kwatch" />
</a>

## Get in touch!

Feel free to chat with us on [Discord](https://discord.gg/kzJszdKmJ7) if you have questions, or suggestions

## License

kwatch is licensed under [MIT License](LICENSE)
