package generator

var ClusterSourceTemplate = `<source>
<source>
@type tail
path {{.Path}}
pos_file /var/fluentd/etc/config/cluster/custom/cluster.pos
tag cluster-custom.*
format {{.Format}}
time_format %Y-%m-%dT%H:%M:%S
</source>
`

var ProjectSourceTemplate = `<source>
@type tail
path {{.Path}}
pos_file /var/fluentd/etc/config/project/custom/project.pos
tag project-custom.{{.Project}}.*
format {{.Format}}
time_format %Y-%m-%dT%H:%M:%S
</source>
`
