# drone-nexus-publish

Drone plugin to publish artifacts to Nexus Repository Manager.

## Docker

Build the Docker image with the following commands:

```
docker buildx build -t DOCKER_ORG/drone-nexus-publish --platform linux/amd64 .
```

Please note incorrectly building the image for the correct x64 linux and with
CGO disabled will result in an error when running the Docker image:

```
docker: Error response from daemon: Container command
'/bin/drone-nexus-publish' not found or does not exist..
```

## Usage for Single file Upload

```bash
docker run --rm \
  -e PLUGIN_USERNAME=${username} \
  -e PLUGIN_PASSWORD=${password} \
  -e PLUGIN_SERVER_URL=http://nexus-publish.server \
  -e PLUGIN_FILENAME=./target/example.jar \
  -e PLUGIN_FORMAT=maven2 \
  -e PLUGIN_REPOSITORY=maven-releases \
  -e PLUGIN_ATTRIBUTES="-CgroupId=org.testing -CartifactId=example -Cversion=1.0 -Aextension=jar -Aclassifier=bin" \
  -v $(pwd):$(pwd) \
  -w $(pwd) \
  harnesscommunity/drone-nexus-publish
```



In Harness CI, YAML for single file Upload
```yaml
              - step:
                  type: Plugin
                  name: Plugin_1
                  identifier: Plugin_1
                  spec:
                    connectorRef: harnessnew
                    image: harnesscommunity/publish-nexus-repository:1.1.1
                    settings:
                      username: deploy-user
                      password: testing-nexus
                      server_url: http://nexus-publish.server
                      filename: ./target/example.jar
                      format: maven2
                      repository: maven-releases
                      attributes: "-CgroupId=org.testing -CartifactId=example -Cversion=1.0 -Aextension=jar -Aclassifier=bin"
```

## Usage for Multi file Upload
```bash
docker run --rm --network host \
    -e PLUGIN_NEXUS_VERSION='nexus3' \
    -e PLUGIN_SERVER_URL='43.204.190.241:8081' \
    -e PLUGIN_USERNAME='nexususer01' \
    -e PLUGIN_PASSWORD='some!secret@abc' \
    -e PLUGIN_FORMAT='maven2' \
    -e PLUGIN_GROUP_ID='test01' \
    -e PLUGIN_PROTOCOL='http' \
    -e PLUGIN_ARTIFACTS='[{"file": "file1.yaml", "classifier": "bin", "groupId": "test", "artifactId": "config-yaml-1", "type": "yaml", "version": "1"}, {"file": "file2.yaml", "classifier": "src", "groupId": "test", "artifactId": "all-config-yaml-2", "type": "yaml", "version": "2"}]' \
    -e PLUGIN_REPOSITORY='stage-dev-repo' \
      -v $(pwd):$(pwd) \
      -w $(pwd) \
    plugins/nexus-publish:latest
```

In Harnes CI, YAML for multi file Upload
```yaml
- step:
    type: Plugin
    name: Plugin_1
    identifier: Plugin_1
    spec:
      connectorRef: Docker_Hub_Anonymous
      image: plugins/nexus-publish:latest
      settings:
        nexus_version: nexus3
        server_url: 43.204.190.241:8081
        username: <+secrets.getValue("nexus_plugin_username")>
        password: <+secrets.getValue("nexus_plugin_password")>
        format: maven2
        repository: stage-dev-repo
        group_id: test01
        protocol: http
        artifacts:
          - file: file1.yaml
            classifier: bin
            groupId: test
            artifactId: config-yaml-1
            type: yaml
            version: "1"
          - file: file2.yaml
            classifier: src
            groupId: test
            artifactId: all-config-yaml-2
            type: yaml
            version: "2"
```
