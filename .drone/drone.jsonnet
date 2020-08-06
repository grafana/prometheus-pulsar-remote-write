// build_image is the image used by default to build make targets.
local build_image = std.extVar('BUILD_IMAGE');

// make defines the common configuration for a Drone step that builds a make target.
local make(target) = {
  name: 'make %s' % target,
  image: build_image,
  commands: [
    'make %s' % target,
  ],
};

// pipeline defines an empty Drone pipeline.
local pipeline(name) = {
  kind: 'pipeline',
  name: name,
  steps: [],
};


[
  pipeline('prelude') {
    steps: [
      make('-B .drone/drone.yml') {
        commands+: ['git diff --exit-code'],
      },
    ],
  },

  pipeline('check') {
    depends_on: ['prelude'],
    steps: [
      make('lint'),
      make('test'),
      make('bench'),
      make('binaries'),
    ],
  },

  pipeline('release') {
    depends_on: ['check'],
    steps: [
      make('binaries'),
      make('shas'),
      {
        name: 'github-release',
        image: 'plugins/github-release',
        settings: {
          title: '${DRONE_TAG}',
          api_key: { from_secret: 'github_token' },
          files: ['dist/*'],
        },
      },
    ],
    trigger: {
      ref: ['refs/tags/v*'],
    },
  },

  pipeline('build-image') {
    depends_on: ['prelude'],
    steps: [
      {
        name: 'build',
        image: 'plugins/docker',
        settings: {
          dockerfile: 'build-image/Dockerfile',
          repo: 'grafana/prometheus-pulsar-remote-write-build-image',
          password: { from_secret: 'docker_password' },
          username: { from_secret: 'docker_username' },
          tags: ['latest', '${DRONE_BRANCH}', '${DRONE_COMMIT_SHA:0:8}'],
        },
      },
    ],
    trigger: {
      ref: ['refs/heads/master'],
    },
  },
]
