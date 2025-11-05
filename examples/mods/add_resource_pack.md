# Adding a Resource Pack

1. Extract the resource pack to your mounted `resource_packs` directory, taking note of the UUID and version in the `header` object. For example: `34a8c997-79c0-4e17-b92e-98817c688db3` and `[0,0,1]`.

```
# manifest.json
{
	"format_version": 2,
	"header": {
		"name": "Some Resource Pack Name",
		"description": "Some Resource Pack description that goes into more detail about what this Resource Pack does.",
		"uuid": "34a8c997-79c0-4e17-b92e-98817c688db3",
		"version": [
			0,
			0,
			1
		],
		"min_engine_version": [
			1,
			21,
			100
		]
	},
	"modules": [
		{
			"type": "resources",
			"uuid": "523751a8-6370-4992-8310-d21a1fa83c0c",
			"version": [
				1,
				0,
				0
			]
		}
	],
	"metadata": {
		"authors": [
			"Jane Q. Public"
		]
	}
}
```

2. Update (or create) a `world_resource_packs.json` file (for example: `./worlds/Bedrock level/world_resource_packs.json`) with the information from the resource pack's UUID and version from `manfest.json`.

    NOTE: Each resource pack will have its own entry in this JSON file.

```
# world_resource_packs.json
[
    {
        "pack_id": "34a8c997-79c0-4e17-b92e-98817c688db3",
        "version": [0, 0, 1]
    }
]
```