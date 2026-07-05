package analytics

import (
	"fmt"
	"io"
	"sort"
)

type Report struct {
	Name        string
	Description string
	Cypher      string
}

var reports = []Report{
	{
		Name:        "top-clients",
		Description: "Clients with the highest aggregate query volume.",
		Cypher: `MATCH (client:Client)-[queried:QUERIED]->(:Domain)
RETURN client.ip AS client,
       sum(queried.count) AS queries,
       sum(coalesce(queried.nxCount, 0)) AS nxQueries,
       max(queried.lastSeen) AS lastSeen
ORDER BY queries DESC, lastSeen DESC
LIMIT 25;`,
	},
	{
		Name:        "top-domains",
		Description: "Domains with the highest aggregate query volume.",
		Cypher: `MATCH (:Client)-[queried:QUERIED]->(domain:Domain)
RETURN domain.name AS domain,
       sum(queried.count) AS queries,
       count(queried) AS clients,
       sum(coalesce(queried.nxCount, 0)) AS nxQueries,
       max(queried.lastSeen) AS lastSeen
ORDER BY queries DESC, clients DESC
LIMIT 25;`,
	},
	{
		Name:        "top-nxdomain-clients",
		Description: "Clients producing the most NXDOMAIN responses.",
		Cypher: `MATCH (client:Client)-[queried:QUERIED]->(:Domain)
WITH client, sum(coalesce(queried.nxCount, 0)) AS nxQueries, sum(queried.count) AS queries
WHERE nxQueries > 0
RETURN client.ip AS client,
       nxQueries,
       queries,
       round(toFloat(nxQueries) / queries * 10000) / 100 AS nxPercent
ORDER BY nxQueries DESC, nxPercent DESC
LIMIT 25;`,
	},
	{
		Name:        "top-nxdomain-domains",
		Description: "Domains receiving the most NXDOMAIN responses.",
		Cypher: `MATCH (:Client)-[queried:QUERIED]->(domain:Domain)
WITH domain, sum(coalesce(queried.nxCount, 0)) AS nxQueries, sum(queried.count) AS queries, count(queried) AS clients
WHERE nxQueries > 0
RETURN domain.name AS domain,
       nxQueries,
       queries,
       clients,
       round(toFloat(nxQueries) / queries * 10000) / 100 AS nxPercent
ORDER BY nxQueries DESC, clients DESC
LIMIT 25;`,
	},
	{
		Name:        "new-domains-24h",
		Description: "Domains first seen during the last 24 hours.",
		Cypher: `MATCH (domain:Domain)
WHERE domain.firstSeen >= datetime() - duration({hours: 24})
OPTIONAL MATCH (:Client)-[queried:QUERIED]->(domain)
RETURN domain.name AS domain,
       domain.firstSeen AS firstSeen,
       domain.lastSeen AS lastSeen,
       coalesce(sum(queried.count), 0) AS queries,
       count(queried) AS clients
ORDER BY firstSeen DESC
LIMIT 50;`,
	},
	{
		Name:        "single-client-domains",
		Description: "Domains seen from exactly one client.",
		Cypher: `MATCH (client:Client)-[queried:QUERIED]->(domain:Domain)
WITH domain, collect(client.ip) AS clients, sum(queried.count) AS queries, max(queried.lastSeen) AS lastSeen
WHERE size(clients) = 1
RETURN domain.name AS domain,
       clients[0] AS onlyClient,
       queries,
       lastSeen
ORDER BY lastSeen DESC, queries DESC
LIMIT 50;`,
	},
	{
		Name:        "secondary-heavy-clients",
		Description: "Clients with unusually high secondary DNS usage.",
		Cypher: `MATCH (client:Client)-[:ASKED]->(event:DnsEvent)
WITH client,
     count(event) AS total,
     sum(CASE WHEN event.serverRole = "secondary" THEN 1 ELSE 0 END) AS secondary
WHERE total >= 10 AND toFloat(secondary) / total >= 0.5
RETURN client.ip AS client,
       total,
       secondary,
       round(toFloat(secondary) / total * 10000) / 100 AS secondaryPercent
ORDER BY secondaryPercent DESC, secondary DESC
LIMIT 25;`,
	},
	{
		Name:        "client-query-increase",
		Description: "Clients with elevated query volume in the last hour compared with the previous 24 hours.",
		Cypher: `MATCH (event:DnsEvent)
WHERE event.timestamp >= datetime() - duration({hours: 25})
WITH event.clientIP AS client,
     sum(CASE WHEN event.timestamp >= datetime() - duration({hours: 1}) THEN 1 ELSE 0 END) AS lastHour,
     sum(CASE WHEN event.timestamp < datetime() - duration({hours: 1}) THEN 1 ELSE 0 END) AS previous24h
WITH client, lastHour, previous24h, toFloat(previous24h) / 24 AS hourlyBaseline
WHERE lastHour >= 20 AND (hourlyBaseline = 0 OR lastHour >= hourlyBaseline * 3)
RETURN client,
       lastHour,
       previous24h,
       round(hourlyBaseline * 100) / 100 AS hourlyBaseline,
       CASE WHEN hourlyBaseline = 0 THEN null ELSE round(toFloat(lastHour) / hourlyBaseline * 100) / 100 END AS multiplier
ORDER BY lastHour DESC
LIMIT 25;`,
	},
	{
		Name:        "domain-query-increase",
		Description: "Domains with elevated query volume in the last hour compared with the previous 24 hours.",
		Cypher: `MATCH (event:DnsEvent)
WHERE event.timestamp >= datetime() - duration({hours: 25})
WITH event.queryName AS domain,
     sum(CASE WHEN event.timestamp >= datetime() - duration({hours: 1}) THEN 1 ELSE 0 END) AS lastHour,
     sum(CASE WHEN event.timestamp < datetime() - duration({hours: 1}) THEN 1 ELSE 0 END) AS previous24h
WITH domain, lastHour, previous24h, toFloat(previous24h) / 24 AS hourlyBaseline
WHERE lastHour >= 20 AND (hourlyBaseline = 0 OR lastHour >= hourlyBaseline * 3)
RETURN domain,
       lastHour,
       previous24h,
       round(hourlyBaseline * 100) / 100 AS hourlyBaseline,
       CASE WHEN hourlyBaseline = 0 THEN null ELSE round(toFloat(lastHour) / hourlyBaseline * 100) / 100 END AS multiplier
ORDER BY lastHour DESC
LIMIT 25;`,
	},
	{
		Name:        "first-last-seen",
		Description: "FirstSeen and LastSeen report for domains.",
		Cypher: `MATCH (domain:Domain)
OPTIONAL MATCH (:Client)-[queried:QUERIED]->(domain)
RETURN domain.name AS domain,
       domain.firstSeen AS firstSeen,
       domain.lastSeen AS lastSeen,
       coalesce(sum(queried.count), 0) AS queries,
       count(queried) AS clients
ORDER BY lastSeen DESC
LIMIT 50;`,
	},
}

func ListReports() []Report {
	result := append([]Report(nil), reports...)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func FindReport(name string) (Report, bool) {
	for _, report := range reports {
		if report.Name == name {
			return report, true
		}
	}
	return Report{}, false
}

func PrintReports(w io.Writer, name string) error {
	if name != "" {
		report, ok := FindReport(name)
		if !ok {
			return fmt.Errorf("unknown analytics report %q", name)
		}
		printReport(w, report)
		return nil
	}

	for idx, report := range ListReports() {
		if idx > 0 {
			fmt.Fprintln(w)
		}
		printReport(w, report)
	}
	return nil
}

func printReport(w io.Writer, report Report) {
	fmt.Fprintf(w, "# %s\n", report.Name)
	fmt.Fprintf(w, "# %s\n", report.Description)
	fmt.Fprintln(w, report.Cypher)
}
