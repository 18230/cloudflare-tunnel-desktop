export namespace main {

	export class Route {
	    id: string;
	    hostname: string;
	    serviceProtocol: string;
	    serviceHost: string;
	    servicePort: number;
	    enabled: boolean;

	    static createFrom(source: any = {}) {
	        return new Route(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.hostname = source["hostname"];
	        this.serviceProtocol = source["serviceProtocol"];
	        this.serviceHost = source["serviceHost"];
	        this.servicePort = source["servicePort"];
	        this.enabled = source["enabled"];
	    }
	}
	export class AppConfig {
	    accountId: string;
	    zoneId: string;
	    rootDomain: string;
	    tunnelId: string;
	    tunnelName: string;
	    protocol: string;
	    autoRestart: boolean;
	    authType: string;
	    authEmail: string;
	    apiToken: string;
	    tunnelToken: string;
	    routes: Route[];

	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accountId = source["accountId"];
	        this.zoneId = source["zoneId"];
	        this.rootDomain = source["rootDomain"];
	        this.tunnelId = source["tunnelId"];
	        this.tunnelName = source["tunnelName"];
	        this.protocol = source["protocol"];
	        this.autoRestart = source["autoRestart"];
	        this.authType = source["authType"];
	        this.authEmail = source["authEmail"];
	        this.apiToken = source["apiToken"];
	        this.tunnelToken = source["tunnelToken"];
	        this.routes = this.convertValues(source["routes"], Route);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CloudflareAccount {
	    id: string;
	    name: string;

	    static createFrom(source: any = {}) {
	        return new CloudflareAccount(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class CloudflareTunnel {
	    id: string;
	    name: string;
	    status: string;
	    token: string;
	    conns_active_at: string;

	    static createFrom(source: any = {}) {
	        return new CloudflareTunnel(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.token = source["token"];
	        this.conns_active_at = source["conns_active_at"];
	    }
	}
	export class CloudflareZone {
	    id: string;
	    name: string;
	    status: string;
	    account: CloudflareAccount;

	    static createFrom(source: any = {}) {
	        return new CloudflareZone(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.account = this.convertValues(source["account"], CloudflareAccount);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CloudflareDiscoveryResult {
	    config: AppConfig;
	    accounts: CloudflareAccount[];
	    zones: CloudflareZone[];
	    tunnels: CloudflareTunnel[];
	    messages: string[];

	    static createFrom(source: any = {}) {
	        return new CloudflareDiscoveryResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], AppConfig);
	        this.accounts = this.convertValues(source["accounts"], CloudflareAccount);
	        this.zones = this.convertValues(source["zones"], CloudflareZone);
	        this.tunnels = this.convertValues(source["tunnels"], CloudflareTunnel);
	        this.messages = source["messages"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}


	export class CloudflaredInstallStatus {
	    installed: boolean;
	    installing: boolean;
	    path: string;
	    version: string;
	    status: string;
	    error: string;
	    logs: string[];

	    static createFrom(source: any = {}) {
	        return new CloudflaredInstallStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.installing = source["installing"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.logs = source["logs"];
	    }
	}
	export class CredentialsInput {
	    authType: string;
	    authEmail: string;
	    apiToken: string;
	    tunnelToken: string;

	    static createFrom(source: any = {}) {
	        return new CredentialsInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.authType = source["authType"];
	        this.authEmail = source["authEmail"];
	        this.apiToken = source["apiToken"];
	        this.tunnelToken = source["tunnelToken"];
	    }
	}
	export class LogEntry {
	    // Go type: time
	    time: any;
	    level: string;
	    source: string;
	    message: string;
	    category: string;

	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.level = source["level"];
	        this.source = source["source"];
	        this.message = source["message"];
	        this.category = source["category"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

	export class RouteInput {
	    id: string;
	    hostname: string;
	    serviceProtocol: string;
	    serviceHost: string;
	    servicePort: number;
	    enabled: boolean;

	    static createFrom(source: any = {}) {
	        return new RouteInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.hostname = source["hostname"];
	        this.serviceProtocol = source["serviceProtocol"];
	        this.serviceHost = source["serviceHost"];
	        this.servicePort = source["servicePort"];
	        this.enabled = source["enabled"];
	    }
	}
	export class RuntimeStatus {
	    configured: boolean;
	    authType: string;
	    apiTokenSet: boolean;
	    tunnelTokenSet: boolean;
	    cloudflaredPath: string;
	    cloudflaredVersion: string;
	    running: boolean;
	    pid: number;
	    protocol: string;
	    tunnelStatus: string;
	    uptimeSeconds: number;
	    lastError: string;
	    lastEvent: string;
	    autoRestart: boolean;
	    restartAttempts: number;
	    routeCount: number;
	    hasTunnelId: boolean;

	    static createFrom(source: any = {}) {
	        return new RuntimeStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.authType = source["authType"];
	        this.apiTokenSet = source["apiTokenSet"];
	        this.tunnelTokenSet = source["tunnelTokenSet"];
	        this.cloudflaredPath = source["cloudflaredPath"];
	        this.cloudflaredVersion = source["cloudflaredVersion"];
	        this.running = source["running"];
	        this.pid = source["pid"];
	        this.protocol = source["protocol"];
	        this.tunnelStatus = source["tunnelStatus"];
	        this.uptimeSeconds = source["uptimeSeconds"];
	        this.lastError = source["lastError"];
	        this.lastEvent = source["lastEvent"];
	        this.autoRestart = source["autoRestart"];
	        this.restartAttempts = source["restartAttempts"];
	        this.routeCount = source["routeCount"];
	        this.hasTunnelId = source["hasTunnelId"];
	    }
	}
	export class SettingsInput {
	    accountId: string;
	    zoneId: string;
	    rootDomain: string;
	    tunnelId: string;
	    tunnelName: string;
	    protocol: string;
	    autoRestart: boolean;

	    static createFrom(source: any = {}) {
	        return new SettingsInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accountId = source["accountId"];
	        this.zoneId = source["zoneId"];
	        this.rootDomain = source["rootDomain"];
	        this.tunnelId = source["tunnelId"];
	        this.tunnelName = source["tunnelName"];
	        this.protocol = source["protocol"];
	        this.autoRestart = source["autoRestart"];
	    }
	}
	export class SyncResult {
	    config: AppConfig;
	    messages: string[];

	    static createFrom(source: any = {}) {
	        return new SyncResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], AppConfig);
	        this.messages = source["messages"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TunnelRouteOverview {
	    tunnelId: string;
	    tunnelName: string;
	    tunnelStatus: string;
	    hostname: string;
	    serviceProtocol: string;
	    serviceHost: string;
	    servicePort: number;
	    enabled: boolean;
	    source: string;

	    static createFrom(source: any = {}) {
	        return new TunnelRouteOverview(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tunnelId = source["tunnelId"];
	        this.tunnelName = source["tunnelName"];
	        this.tunnelStatus = source["tunnelStatus"];
	        this.hostname = source["hostname"];
	        this.serviceProtocol = source["serviceProtocol"];
	        this.serviceHost = source["serviceHost"];
	        this.servicePort = source["servicePort"];
	        this.enabled = source["enabled"];
	        this.source = source["source"];
	    }
	}
	export class TunnelRouteOverviewResult {
	    routes: TunnelRouteOverview[];
	    messages: string[];

	    static createFrom(source: any = {}) {
	        return new TunnelRouteOverviewResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.routes = this.convertValues(source["routes"], TunnelRouteOverview);
	        this.messages = source["messages"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

