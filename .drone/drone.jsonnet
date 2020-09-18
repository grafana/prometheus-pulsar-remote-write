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

// docker can be used to build docker images.
local docker(repo) = {
  name: 'docker %s' % repo,
  image: 'plugins/docker',
  settings: {
    repo: repo,
    password: { from_secret: 'docker_password' },
    username: { from_secret: 'docker_username' },
    tags: ['latest', '${DRONE_COMMIT_SHA:0:8}'],
  },
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
      docker('grafana/prometheus-pulsar-remote-write') {
        settings+: {
          tags+: ['${DRONE_TAG}'],
        },
      },
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
      docker('grafana/prometheus-pulsar-remote-write-build-image') {
        settings+: {
          dockerfile: 'build-image/Dockerfile',
          tags+: ['${DRONE_BRANCH}'],
        },
      },
    ],
    trigger: {
      ref: ['refs/heads/master'],
    },
  },
]
