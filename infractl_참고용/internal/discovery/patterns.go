// Package discovery
// File: patterns.go
// Description: 빌트인 서비스 패턴 정의 (Oracle, MySQL, PostgreSQL, Tomcat, WebLogic)
// Responsibility: 서비스 타입별 프로세스 정규식, 기본 포트, 설정 파일 경로를 정의

package discovery

// BuiltinPatterns는 빌트인 서비스 패턴 목록이다.
// 패턴 순서는 확신도 계산에 영향을 주지 않는다.
var BuiltinPatterns = []ServicePattern{
	{
		Type:         ServiceOracle,
		ProcessRegex: `ora_pmon_(\w+)`,
		DefaultPorts: []int{1521},
		ConfigFiles:  []string{"/etc/oratab", "/var/opt/oracle/oratab"},
	},
	{
		Type:         ServiceMySQL,
		ProcessRegex: `(?i)mysqld`,
		DefaultPorts: []int{3306},
		ConfigFiles:  []string{"/etc/my.cnf", "/etc/mysql/my.cnf", "/etc/mysql/mysql.conf.d/mysqld.cnf"},
	},
	{
		Type:         ServicePostgreSQL,
		ProcessRegex: `postgres:\s+`,
		DefaultPorts: []int{5432},
		ConfigFiles:  []string{"/var/lib/pgsql/data/pg_hba.conf", "/etc/postgresql/pg_hba.conf"},
	},
	{
		Type:         ServiceTomcat,
		ProcessRegex: `java.*(?i)(catalina|tomcat)`,
		DefaultPorts: []int{8080, 8443, 8009},
		ConfigFiles:  []string{},
	},
	{
		Type:         ServiceWebLogic,
		ProcessRegex: `java.*weblogic\.Server`,
		DefaultPorts: []int{7001, 7002},
		ConfigFiles:  []string{},
	},
	{
		Type:         ServiceEtcd,
		ProcessRegex: `etcd\b`,
		DefaultPorts: []int{2379, 2380, 4001},
		ConfigFiles:  []string{"/etc/etcd/etcd.conf", "/etc/kubernetes/manifests/etcd.yaml"},
	},
	{
		Type:         ServiceKubeAPI,
		ProcessRegex: `kube-apiserver`,
		DefaultPorts: []int{6443},
		ConfigFiles:  []string{"/etc/kubernetes/manifests/kube-apiserver.yaml"},
	},
	{
		Type:         ServiceKubelet,
		ProcessRegex: `kubelet\b`,
		DefaultPorts: []int{10250, 10248},
		ConfigFiles:  []string{"/var/lib/kubelet/config.yaml", "/etc/kubernetes/kubelet.conf"},
	},
	{
		Type:         ServicePrometheus,
		ProcessRegex: `prometheus\b`,
		DefaultPorts: []int{9090, 9099},
		ConfigFiles:  []string{"/etc/prometheus/prometheus.yml"},
	},
	{
		Type:         ServiceNginx,
		ProcessRegex: `nginx:\s+master`,
		DefaultPorts: []int{80, 443},
		ConfigFiles:  []string{"/etc/nginx/nginx.conf", "/usr/local/nginx/conf/nginx.conf"},
	},
	{
		Type:         ServiceApache,
		ProcessRegex: `(?i)httpd|apache2`,
		DefaultPorts: []int{80, 443},
		ConfigFiles:  []string{"/etc/httpd/conf/httpd.conf", "/etc/apache2/apache2.conf"},
	},
	{
		Type:         ServiceHAProxy,
		ProcessRegex: `haproxy\b`,
		DefaultPorts: []int{80, 443, 1080, 1936},
		ConfigFiles:  []string{"/etc/haproxy/haproxy.cfg"},
	},
	{
		Type:         ServiceTraefik,
		ProcessRegex: `traefik\b`,
		DefaultPorts: []int{80, 443, 8080},
		ConfigFiles:  []string{"/etc/traefik/traefik.yml", "/etc/traefik/traefik.toml"},
	},
}

// ConfidenceWeights는 각 증거 유형의 확신도 가중치이다.
const (
	WeightProcess    = 0.5 // 프로세스 매칭
	WeightPort       = 0.2 // 포트 매칭
	WeightConfigFile = 0.3 // 설정 파일 존재
)
