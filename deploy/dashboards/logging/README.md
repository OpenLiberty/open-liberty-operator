# Sample Kibana dashboards for Open Liberty
Here you can find Kibana dashboards to use with JSON logs from Liberty servers.

If you are using AppNav, please use the dashboards under the /AppNav directory.

## open-liberty-kibana\<version\>-traffic

Prior to Open Liberty 20.0.0.8, access logs that were included in the JSON logs used a fixed set of fields. Open Liberty 20.0.0.8 adds the option for you to use the fields specified in the `accessLogging` `logFormat` attribute in the JSON logs.

The traffic dashboard depends on the default access log fields. If you want to use our traffic dashboard, include the default set of access log fields in your access log format `('%h %H %A%B %m %p %q %{R}W %s %U')`

You can also build your own custom dashboards (or modify ours) in Kibana if you want to take advantage of the non-default fields.
