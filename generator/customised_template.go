package generator

var ClusterSourceTemplate = `<source>
@type tail
path {{.Path}}
pos_file {{.ClusterPosPath}}
tag tmp-cluster-custom.*
format {{.Format}}
</source>
`

var ProjectSourceTemplate = `<source>
@type tail
path {{.Path}}
pos_file {{.ProjectPosPath}}
tag tmp-project-custom.*
format {{.Format}}
</source>
`
