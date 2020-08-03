// build_image is the image used by default to build make targets.
local build_image = std.extVar('BUILD_IMAGE');

// make defines the common configuration for a Drone step that builds a make target.
local make(target) = {
  name: target,
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
  pipeline('check') {
    steps: [
      make('test'),
      make('bench'),
      make('build'),
    ],
  },

  pipeline('release') {
    depends_on: ['check'],
    steps: [
      make('build'),
      make('prometheus-pulsar-remote-write.sha256'),
      {
        name: 'github-release',
        image: 'plugins/github-release',
        settings: {
          title: '${DRONE_TAG}',
          api_key: { from_secret: 'github_token' },
          files: ['prometheus-pulsar-remote-write', 'prometheus-pulsar-remote-write.sha256'],
        },
      },
    ],
    trigger: {
      event: ['tag'],
    },
  },
]
