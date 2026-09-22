# Invalid configuration fixtures

These files are inputs for configuration rejection scenarios. They contain no
credentials and must only be applied to a disposable Kwatch installation. A
negative scenario must assert clean rejection, absence of a panic or secret
leak, and successful recovery after the last valid configuration is restored.
