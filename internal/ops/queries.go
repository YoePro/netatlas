package ops

import (
	"fmt"
	"io"
	"sort"
)

type Query struct {
	Name        string
	Description string
	Cypher      string
}

var operationalQueries = []Query{
	{
		Name:        "events-per-server",
		Description: "Count DnsEvent observations per DNS server.",
		Cypher: `MATCH (server:DnsServer)-[:OBSERVED]->(event:DnsEvent)
RETURN server.name AS server, server.role AS role, count(event) AS events
ORDER BY events DESC, server ASC;`,
	},
	{
		Name:        "unique-clients",
		Description: "Count unique clients.",
		Cypher: `MATCH (client:Client)
RETURN count(client) AS uniqueClients;`,
	},
	{
		Name:        "unique-devices",
		Description: "Count persistent Device entities.",
		Cypher: `MATCH (device:Device)
RETURN count(device) AS uniqueDevices;`,
	},
	{
		Name:        "device-client-map",
		Description: "Show Device entities and their observed client IPs.",
		Cypher: `MATCH (device:Device)-[:HAS_CLIENT]->(client:Client)
RETURN device.key AS device,
       device.primaryIP AS primaryIP,
       collect(client.ip) AS clientIPs,
       device.identitySource AS identitySource,
       device.firstSeen AS firstSeen,
       device.lastSeen AS lastSeen
ORDER BY lastSeen DESC
LIMIT 50;`,
	},
	{
		Name:        "unenriched-devices",
		Description: "Show devices without inferred OS, type, or software.",
		Cypher: `MATCH (device:Device)
WHERE NOT (device)-[:LIKELY_RUNNING]->(:OperatingSystem)
  AND NOT (device)-[:LIKELY_IS]->(:DeviceType)
  AND NOT (device)-[:LIKELY_HAS]->(:Software)
RETURN device.key AS device,
       device.primaryIP AS primaryIP,
       device.firstSeen AS firstSeen,
       device.lastSeen AS lastSeen
ORDER BY lastSeen DESC
LIMIT 50;`,
	},
	{
		Name:        "unique-domains",
		Description: "Count unique domains.",
		Cypher: `MATCH (domain:Domain)
RETURN count(domain) AS uniqueDomains;`,
	},
	{
		Name:        "newest-events",
		Description: "Show newest DNS events.",
		Cypher: `MATCH (event:DnsEvent)
RETURN event.timestamp AS timestamp,
       event.serverName AS server,
       event.clientIP AS client,
       event.queryName AS domain,
       event.queryType AS queryType,
       event.responseCode AS responseCode
ORDER BY timestamp DESC
LIMIT 25;`,
	},
	{
		Name:        "top-client-domain",
		Description: "Show top Client-Domain aggregate relationships.",
		Cypher: `MATCH (client:Client)-[queried:QUERIED]->(domain:Domain)
RETURN client.ip AS client,
       domain.name AS domain,
       queried.count AS count,
       queried.nxCount AS nxCount,
       queried.queryTypes AS queryTypes,
       queried.serverSeenOn AS serverSeenOn,
       queried.firstSeen AS firstSeen,
       queried.lastSeen AS lastSeen,
       queried.lastResponseCode AS lastResponseCode
ORDER BY count DESC, lastSeen DESC
LIMIT 25;`,
	},
	{
		Name:        "duplicate-protection",
		Description: "Check whether any rawHash appears more than once.",
		Cypher: `MATCH (event:DnsEvent)
WITH event.rawHash AS rawHash, count(event) AS copies
WHERE copies > 1
RETURN rawHash, copies
ORDER BY copies DESC
LIMIT 25;`,
	},
	{
		Name:        "primary-secondary-split",
		Description: "Show observations split by server role.",
		Cypher: `MATCH (event:DnsEvent)
RETURN event.serverRole AS serverRole, count(event) AS events
ORDER BY events DESC;`,
	},
	{
		Name:        "aggregate-vs-events",
		Description: "Compare event count with aggregate relationship totals.",
		Cypher: `MATCH (event:DnsEvent)
WITH count(event) AS dnsEvents
MATCH ()-[queried:QUERIED]->()
RETURN dnsEvents,
       sum(queried.count) AS aggregateCount,
       dnsEvents - sum(queried.count) AS difference;`,
	},
	{
		Name:        "top-nxdomain",
		Description: "Show Client-Domain aggregates with the most NXDOMAIN responses.",
		Cypher: `MATCH (client:Client)-[queried:QUERIED]->(domain:Domain)
WHERE coalesce(queried.nxCount, 0) > 0
RETURN client.ip AS client,
       domain.name AS domain,
       queried.nxCount AS nxCount,
       queried.count AS count,
       queried.lastSeen AS lastSeen
ORDER BY nxCount DESC, count DESC
LIMIT 25;`,
	},
}

func ListQueries() []Query {
	queries := append([]Query(nil), operationalQueries...)
	queries = append(queries, enrichmentQueries...)
	sort.Slice(queries, func(i, j int) bool {
		return queries[i].Name < queries[j].Name
	})
	return queries
}

func FindQuery(name string) (Query, bool) {
	for _, query := range append(append([]Query(nil), operationalQueries...), enrichmentQueries...) {
		if query.Name == name {
			return query, true
		}
	}
	return Query{}, false
}

func PrintQueries(w io.Writer, name string) error {
	if name != "" {
		query, ok := FindQuery(name)
		if !ok {
			return fmt.Errorf("unknown operational query %q", name)
		}
		printQuery(w, query)
		return nil
	}

	for idx, query := range ListQueries() {
		if idx > 0 {
			fmt.Fprintln(w)
		}
		printQuery(w, query)
	}
	return nil
}

func printQuery(w io.Writer, query Query) {
	fmt.Fprintf(w, "# %s\n", query.Name)
	fmt.Fprintf(w, "# %s\n", query.Description)
	fmt.Fprintln(w, query.Cypher)
}
