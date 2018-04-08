package generator

var ClusterSourceTemplate = `<source>
@type tail
path {{.Path}}
pos_file {{.ClusterPosPath}}
tag cluster-custom.*
format {{.Format}}
</source>
`

var ProjectSourceTemplate = `<source>
@type tail
path {{.Path}}
pos_file {{.ProjectPosPath}}
tag project-custom.{{.Project}}.*
format {{.Format}}
</source>
`
