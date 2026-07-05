package ops

var enrichmentQueries = []Query{
	{
		Name:        "merge-device-os-evidence",
		Description: "Template for recording operating system evidence on a Device.",
		Cypher: `MATCH (device:Device {key: $deviceKey})
MERGE (os:OperatingSystem {name: $name})
MERGE (device)-[rel:LIKELY_RUNNING]->(os)
  ON CREATE SET rel.firstSeen = $timestamp,
                rel.score = 0,
                rel.evidenceCount = 0,
                rel.evidenceHashes = []
SET rel.lastSeen = $timestamp,
    rel.score = coalesce(rel.score, 0) + $score,
    rel.confidence = CASE
      WHEN coalesce(rel.score, 0) + $score >= 70 THEN "high"
      WHEN coalesce(rel.score, 0) + $score >= 40 THEN "medium"
      ELSE "low"
    END,
    rel.evidenceCount = coalesce(rel.evidenceCount, 0) + 1,
    rel.evidenceHashes = CASE
      WHEN $evidenceHash IN coalesce(rel.evidenceHashes, []) THEN coalesce(rel.evidenceHashes, [])
      ELSE coalesce(rel.evidenceHashes, []) + $evidenceHash
    END;`,
	},
	{
		Name:        "merge-device-type-evidence",
		Description: "Template for recording device type evidence on a Device.",
		Cypher: `MATCH (device:Device {key: $deviceKey})
MERGE (deviceType:DeviceType {name: $name})
MERGE (device)-[rel:LIKELY_IS]->(deviceType)
  ON CREATE SET rel.firstSeen = $timestamp,
                rel.score = 0,
                rel.evidenceCount = 0,
                rel.evidenceHashes = []
SET rel.lastSeen = $timestamp,
    rel.score = coalesce(rel.score, 0) + $score,
    rel.confidence = CASE
      WHEN coalesce(rel.score, 0) + $score >= 70 THEN "high"
      WHEN coalesce(rel.score, 0) + $score >= 40 THEN "medium"
      ELSE "low"
    END,
    rel.evidenceCount = coalesce(rel.evidenceCount, 0) + 1,
    rel.evidenceHashes = CASE
      WHEN $evidenceHash IN coalesce(rel.evidenceHashes, []) THEN coalesce(rel.evidenceHashes, [])
      ELSE coalesce(rel.evidenceHashes, []) + $evidenceHash
    END;`,
	},
	{
		Name:        "merge-device-software-evidence",
		Description: "Template for recording software evidence on a Device.",
		Cypher: `MATCH (device:Device {key: $deviceKey})
MERGE (software:Software {name: $name})
MERGE (device)-[rel:LIKELY_HAS]->(software)
  ON CREATE SET rel.firstSeen = $timestamp,
                rel.score = 0,
                rel.evidenceCount = 0,
                rel.evidenceHashes = []
SET rel.lastSeen = $timestamp,
    rel.score = coalesce(rel.score, 0) + $score,
    rel.confidence = CASE
      WHEN coalesce(rel.score, 0) + $score >= 70 THEN "high"
      WHEN coalesce(rel.score, 0) + $score >= 40 THEN "medium"
      ELSE "low"
    END,
    rel.evidenceCount = coalesce(rel.evidenceCount, 0) + 1,
    rel.evidenceHashes = CASE
      WHEN $evidenceHash IN coalesce(rel.evidenceHashes, []) THEN coalesce(rel.evidenceHashes, [])
      ELSE coalesce(rel.evidenceHashes, []) + $evidenceHash
    END;`,
	},
	{
		Name:        "merge-device-infrastructure-evidence",
		Description: "Template for recording infrastructure role evidence on a Device.",
		Cypher: `MATCH (device:Device {key: $deviceKey})
MERGE (role:InfrastructureRole {name: $name})
MERGE (device)-[rel:LIKELY_INFRASTRUCTURE]->(role)
  ON CREATE SET rel.firstSeen = $timestamp,
                rel.score = 0,
                rel.evidenceCount = 0,
                rel.evidenceHashes = []
SET rel.lastSeen = $timestamp,
    rel.score = coalesce(rel.score, 0) + $score,
    rel.confidence = CASE
      WHEN coalesce(rel.score, 0) + $score >= 70 THEN "high"
      WHEN coalesce(rel.score, 0) + $score >= 40 THEN "medium"
      ELSE "low"
    END,
    rel.evidenceCount = coalesce(rel.evidenceCount, 0) + 1,
    rel.evidenceHashes = CASE
      WHEN $evidenceHash IN coalesce(rel.evidenceHashes, []) THEN coalesce(rel.evidenceHashes, [])
      ELSE coalesce(rel.evidenceHashes, []) + $evidenceHash
    END;`,
	},
	{
		Name:        "merge-device-vendor-evidence",
		Description: "Template for recording vendor evidence on a Device.",
		Cypher: `MATCH (device:Device {key: $deviceKey})
MERGE (vendor:Vendor {name: $name})
MERGE (device)-[rel:LIKELY_VENDOR]->(vendor)
  ON CREATE SET rel.firstSeen = $timestamp,
                rel.score = 0,
                rel.evidenceCount = 0,
                rel.evidenceHashes = []
SET rel.lastSeen = $timestamp,
    rel.score = coalesce(rel.score, 0) + $score,
    rel.confidence = CASE
      WHEN coalesce(rel.score, 0) + $score >= 70 THEN "high"
      WHEN coalesce(rel.score, 0) + $score >= 40 THEN "medium"
      ELSE "low"
    END,
    rel.evidenceCount = coalesce(rel.evidenceCount, 0) + 1,
    rel.evidenceHashes = CASE
      WHEN $evidenceHash IN coalesce(rel.evidenceHashes, []) THEN coalesce(rel.evidenceHashes, [])
      ELSE coalesce(rel.evidenceHashes, []) + $evidenceHash
    END;`,
	},
}
