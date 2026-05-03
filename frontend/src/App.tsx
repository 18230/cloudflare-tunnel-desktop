import {FormEvent, useEffect, useMemo, useState} from 'react';
import {
    Activity,
    AlertTriangle,
    CircleStop,
    Cloud,
    Copy,
    KeyRound,
    Plus,
    Pencil,
    RefreshCw,
    RotateCw,
    Save,
    Server,
    ShieldCheck,
    Trash2,
    Play,
    ExternalLink,
} from 'lucide-react';
import './App.css';
import {
    AddRoute,
    BindTunnel,
    CreateTunnel,
    DeleteTunnel,
    FetchZones,
    GetCloudflaredInstallStatus,
    GetLogs,
    GetStatus,
    InstallCloudflared,
    ListTunnels,
    LoadConfig,
    PullRoutesFromCloudflare,
    RefreshStatus,
    RemoveRoute,
    RestartTunnel,
    SaveSettings,
    SetCredentials,
    StartTunnel,
    StopTunnel,
    SyncRoutes,
    UpdateRoute,
} from '../wailsjs/go/main/App';
import {BrowserOpenURL, EventsOff, EventsOn} from '../wailsjs/runtime/runtime';

type AppConfig = {
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
};

type Route = {
    id: string;
    hostname: string;
    serviceProtocol: string;
    serviceHost: string;
    servicePort: number;
    enabled: boolean;
};

type RouteFormState = Omit<Route, 'servicePort'> & {
    servicePort: number | '';
};

type RuntimeStatus = {
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
};

type CloudflaredInstallStatus = {
    installed: boolean;
    installing: boolean;
    path: string;
    version: string;
    status: string;
    error: string;
    logs: string[];
};

type LogEntry = {
    time: string;
    level: string;
    source: string;
    message: string;
    category: string;
};

type CloudflareZone = {
    id: string;
    name: string;
    status: string;
    account: {
        id: string;
        name: string;
    };
};

type CloudflareTunnel = {
    id: string;
    name: string;
    status: string;
    token: string;
    conns_active_at: string;
};

const defaultConfig: AppConfig = {
    accountId: '',
    zoneId: '',
    rootDomain: '',
    tunnelId: '',
    tunnelName: '',
    protocol: 'auto',
    autoRestart: true,
    authType: 'api_token',
    authEmail: '',
    apiToken: '',
    tunnelToken: '',
    routes: [],
};

const defaultStatus: RuntimeStatus = {
    configured: false,
    authType: 'api_token',
    apiTokenSet: false,
    tunnelTokenSet: false,
    cloudflaredPath: '',
    cloudflaredVersion: '未知',
    running: false,
    pid: 0,
    protocol: 'auto',
    tunnelStatus: '',
    uptimeSeconds: 0,
    lastError: '',
    lastEvent: '',
    autoRestart: true,
    restartAttempts: 0,
    routeCount: 0,
    hasTunnelId: false,
};

const defaultInstallStatus: CloudflaredInstallStatus = {
    installed: false,
    installing: false,
    path: '',
    version: '未安装',
    status: '正在检测 cloudflared',
    error: '',
    logs: [],
};

const blankRoute: RouteFormState = {
    id: '',
    hostname: '',
    serviceProtocol: 'http',
    serviceHost: 'localhost',
    servicePort: '',
    enabled: true,
};

const helpLinks = {
    accountAndZone: 'https://developers.cloudflare.com/fundamentals/setup/find-account-and-zone-ids/',
    apiToken: 'https://dash.cloudflare.com/profile/api-tokens',
    tunnelList: 'https://one.dash.cloudflare.com/?to=/:account/networks/tunnels',
    tunnelToken: 'https://developers.cloudflare.com/tunnel/advanced/tunnel-tokens/',
    tunnelSetup: 'https://developers.cloudflare.com/tunnel/setup/',
    protocol: 'https://developers.cloudflare.com/tunnel/advanced/run-parameters/#protocol',
};

function App() {
    const [config, setConfig] = useState<AppConfig>(defaultConfig);
    const [settings, setSettings] = useState<AppConfig>(defaultConfig);
    const [credentials, setCredentials] = useState({authType: 'api_token', authEmail: '', apiToken: '', tunnelToken: ''});
    const [routeForm, setRouteForm] = useState<RouteFormState>(blankRoute);
    const [status, setStatus] = useState<RuntimeStatus>(defaultStatus);
    const [installStatus, setInstallStatus] = useState<CloudflaredInstallStatus>(defaultInstallStatus);
    const [logs, setLogs] = useState<LogEntry[]>([]);
    const [availableZones, setAvailableZones] = useState<CloudflareZone[]>([]);
    const [availableTunnels, setAvailableTunnels] = useState<CloudflareTunnel[]>([]);
    const [deleteTunnelId, setDeleteTunnelId] = useState('');
    const [deleteTunnelDNS, setDeleteTunnelDNS] = useState(false);
    const [selectedRouteTunnelId, setSelectedRouteTunnelId] = useState('');
    const [activePanel, setActivePanel] = useState<'config' | 'tunnels' | 'routes' | 'logs'>('config');
    const [message, setMessage] = useState('');
    const [busy, setBusy] = useState('');

    useEffect(() => {
        void bootstrap();
        EventsOn('app:log', (entry: LogEntry) => {
            setLogs((current) => [...current.slice(-199), entry]);
        });
        const timer = window.setInterval(() => {
            void refreshLocalState();
        }, 5000);
        return () => {
            window.clearInterval(timer);
            EventsOff('app:log');
        };
    }, []);

    useEffect(() => {
        if (!installStatus.installing) {
            return;
        }
        const timer = window.setInterval(() => {
            void refreshCloudflaredInstallStatus();
        }, 2500);
        return () => window.clearInterval(timer);
    }, [installStatus.installing]);

    useEffect(() => {
        if ((activePanel === 'tunnels' || activePanel === 'routes') && config.accountId && availableTunnels.length === 0) {
            void handleListTunnels(false);
        }
    }, [activePanel, config.accountId]);

    const uptimeText = useMemo(() => formatDuration(status.uptimeSeconds), [status.uptimeSeconds]);

    async function bootstrap() {
        await withBusy('init', async () => {
            const loaded = await LoadConfig();
            setConfig(loaded);
            setSettings(loaded);
            setSelectedRouteTunnelId(loaded.tunnelId || '');
            setCredentials({
                authType: loaded.authType || 'api_token',
                authEmail: loaded.authEmail || '',
                apiToken: loaded.apiToken || '',
                tunnelToken: loaded.tunnelToken || '',
            });
            setInstallStatus(await GetCloudflaredInstallStatus());
            await refreshLocalState();
        });
    }

    async function refreshLocalState() {
        const [nextStatus, nextLogs] = await Promise.all([GetStatus(), GetLogs()]);
        setStatus(nextStatus);
        setLogs(nextLogs);
    }

    async function refreshCloudflaredInstallStatus() {
        const nextInstallStatus = await GetCloudflaredInstallStatus();
        const wasInstalling = installStatus.installing;
        setInstallStatus(nextInstallStatus);
        if (nextInstallStatus.installed) {
            await refreshLocalState();
            if (wasInstalling) {
                setMessage('cloudflared 安装成功');
            }
        } else if (wasInstalling && nextInstallStatus.error) {
            setMessage(nextInstallStatus.error);
        }
    }

    async function handleInstallCloudflared() {
        await withBusy('installCloudflared', async () => {
            const nextInstallStatus = await InstallCloudflared();
            setInstallStatus(nextInstallStatus);
            if (nextInstallStatus.installed) {
                await refreshLocalState();
                setMessage('cloudflared 已安装');
            } else if (nextInstallStatus.installing) {
                setMessage('cloudflared 正在安装，完成后会自动刷新状态');
            }
        });
    }

    async function handleSaveSettings(event: FormEvent) {
        event.preventDefault();
        await withBusy('settings', async () => {
            const saved = await SaveSettings({
                accountId: settings.accountId,
                zoneId: settings.zoneId,
                rootDomain: settings.rootDomain,
                tunnelId: settings.tunnelId,
                tunnelName: settings.tunnelName,
                protocol: settings.protocol,
                autoRestart: settings.autoRestart,
            });
            setConfig(saved);
            setSettings(saved);
            await refreshLocalState();
            setMessage('基础配置已保存');
        });
    }

    async function handleCredentials(event: FormEvent) {
        event.preventDefault();
        await withBusy('credentials', async () => {
            const nextStatus = await SetCredentials(credentials);
            setStatus(nextStatus);
            setConfig({
                ...config,
                authType: credentials.authType,
                authEmail: credentials.authEmail,
                apiToken: credentials.apiToken,
                tunnelToken: credentials.tunnelToken,
            });
            setSettings({
                ...settings,
                authType: credentials.authType,
                authEmail: credentials.authEmail,
                apiToken: credentials.apiToken,
                tunnelToken: credentials.tunnelToken,
            });
            setMessage('凭据已明文保存到本地配置文件');
        });
    }

    async function handleCreateTunnel() {
        await withBusy('createTunnel', async () => {
            const saved = await SaveSettings({
                accountId: settings.accountId,
                zoneId: settings.zoneId,
                rootDomain: settings.rootDomain,
                tunnelId: settings.tunnelId,
                tunnelName: settings.tunnelName,
                protocol: settings.protocol,
                autoRestart: settings.autoRestart,
            });
            setConfig(saved);
            setSettings(saved);
            const updated = await CreateTunnel(saved.tunnelName || config.tunnelName || 'desktop-tunnel');
            setConfig(updated);
            setSettings(updated);
            setSelectedRouteTunnelId(updated.tunnelId || '');
            setCredentials((current) => ({...current, tunnelToken: updated.tunnelToken || ''}));
            setAvailableTunnels(await ListTunnels());
            await refreshLocalState();
            setMessage('Tunnel 已创建或绑定');
        });
    }

    async function handleFetchZones() {
        await withBusy('fetchZones', async () => {
            const saved = await SaveSettings({
                accountId: settings.accountId,
                zoneId: settings.zoneId,
                rootDomain: settings.rootDomain,
                tunnelId: settings.tunnelId,
                tunnelName: settings.tunnelName,
                protocol: settings.protocol,
                autoRestart: settings.autoRestart,
            });
            setConfig(saved);
            setSettings(saved);
            const zones = await FetchZones();
            setAvailableZones(zones);
            if (zones.length === 1) {
                await applyZone(zones[0], saved, '已自动填入根域名');
                return;
            }
            setMessage(`已获取 ${zones.length} 个根域名，请从下拉框选择`);
        });
    }

    async function handleListTunnels(showMessage = true) {
        await withBusy('listTunnels', async () => {
            const saved = await SaveSettings({
                accountId: settings.accountId,
                zoneId: settings.zoneId,
                rootDomain: settings.rootDomain,
                tunnelId: settings.tunnelId,
                tunnelName: settings.tunnelName,
                protocol: settings.protocol,
                autoRestart: settings.autoRestart,
            });
            setConfig(saved);
            setSettings(saved);
            const tunnels = await ListTunnels();
            setAvailableTunnels(tunnels);
            setDeleteTunnelId('');
            setDeleteTunnelDNS(false);
            if (showMessage) {
                setMessage(tunnels.length > 0 ? `已获取 ${tunnels.length} 个 Tunnel` : '当前账号没有 Tunnel');
            }
        });
    }

    async function applyTunnel(tunnel: CloudflareTunnel) {
        const saved = await BindTunnel(tunnel);
        setConfig(saved);
        setSettings(saved);
        setSelectedRouteTunnelId(tunnel.id);
        setCredentials((current) => ({...current, tunnelToken: saved.tunnelToken || ''}));
        const result = await PullRoutesFromCloudflare();
        setConfig(result.config);
        setSettings(result.config);
        setCredentials((current) => ({...current, tunnelToken: result.config.tunnelToken || ''}));
        setSelectedRouteTunnelId(result.config.tunnelId || tunnel.id);
        setActivePanel('routes');
        await refreshLocalState();
        setMessage(`已设为当前 Tunnel 并读取映射: ${tunnel.name || tunnel.id}`);
    }

    async function handleDeleteTunnel(tunnel: CloudflareTunnel) {
        const expected = tunnel.name || tunnel.id;
        await withBusy(`deleteTunnel-${tunnel.id}`, async () => {
            const saved = await DeleteTunnel(tunnel.id, deleteTunnelDNS);
            setConfig(saved);
            setSettings(saved);
            setAvailableTunnels((current) => current.filter((item) => item.id !== tunnel.id));
            if (saved.tunnelId !== selectedRouteTunnelId) {
                setSelectedRouteTunnelId(saved.tunnelId || '');
            }
            setDeleteTunnelId('');
            setDeleteTunnelDNS(false);
            await refreshLocalState();
            setMessage(deleteTunnelDNS ? `已删除 Tunnel 和对应 DNS 记录: ${expected}` : `已删除 Tunnel: ${expected}`);
        });
    }

    async function applyZone(zone: CloudflareZone, baseSettings = settings, successMessage = '根域名已选择') {
        const nextSettings = {
            ...baseSettings,
            zoneId: zone.id,
            rootDomain: zone.name,
        };
        setSettings(nextSettings);
        const saved = await SaveSettings({
            accountId: nextSettings.accountId,
            zoneId: nextSettings.zoneId,
            rootDomain: nextSettings.rootDomain,
            tunnelId: nextSettings.tunnelId,
            tunnelName: nextSettings.tunnelName,
            protocol: nextSettings.protocol,
            autoRestart: nextSettings.autoRestart,
        });
        setConfig(saved);
        setSettings(saved);
        setMessage(`${successMessage}: ${zone.name}`);
    }

    async function handleRouteSubmit(event: FormEvent) {
        event.preventDefault();
        await withBusy('route', async () => {
            await ensureSelectedRouteTunnel();
            const input = {
                ...routeForm,
                servicePort: routeForm.servicePort === '' ? 0 : Number(routeForm.servicePort),
            };
            const saved = routeForm.id ? await UpdateRoute(input) : await AddRoute(input);
            setConfig(saved);
            setSettings(saved);
            await syncRoutesAfterLocalChange(routeForm.id ? '映射已更新' : '映射已添加');
            setRouteForm(blankRoute);
        });
    }

    async function handleRemoveRoute(route: Route) {
        await withBusy(`remove-${route.id}`, async () => {
            await ensureSelectedRouteTunnel();
            const saved = await RemoveRoute(route.id, true);
            setConfig(saved);
            setSettings(saved);
            await syncRoutesAfterLocalChange('映射已删除');
        });
    }

    // handleRouteTunnelChange 在映射页切换 Tunnel 时立即载入对应远端映射，避免列表和表单归属不一致。
    function handleRouteTunnelChange(tunnelId: string) {
        setSelectedRouteTunnelId(tunnelId);
        setRouteForm(blankRoute);
        if (!tunnelId || tunnelId === config.tunnelId) {
            return;
        }
        const targetTunnel = availableTunnels.find((item) => item.id === tunnelId);
        if (!targetTunnel) {
            setMessage('请选择有效的 Tunnel');
            return;
        }
        void withBusy(`bindRouteTunnel-${tunnelId}`, () => applyTunnel(targetTunnel));
    }

    // syncRoutesAfterLocalChange 在本地映射变更后自动推送远端配置，并保留同步失败提示。
    async function syncRoutesAfterLocalChange(successPrefix: string) {
        try {
            const result = await SyncRoutes();
            setConfig(result.config);
            setSettings(result.config);
            await refreshLocalState();
            const detail = result.messages.length > 0 ? `；${result.messages.join('；')}` : '';
            setMessage(`${successPrefix}并同步 Cloudflare${detail}`);
        } catch (error) {
            await refreshLocalState();
            const reason = error instanceof Error ? error.message : String(error);
            throw new Error(`${successPrefix}，但同步 Cloudflare 失败: ${reason}`);
        }
    }

    // ensureSelectedRouteTunnel 保证映射表单操作前，后端当前 Tunnel 与下拉选择一致。
    async function ensureSelectedRouteTunnel() {
        const targetTunnelId = selectedRouteTunnelId || config.tunnelId;
        if (!targetTunnelId || targetTunnelId === config.tunnelId) {
            return;
        }
        const targetTunnel = availableTunnels.find((item) => item.id === targetTunnelId);
        if (!targetTunnel) {
            throw new Error('请选择有效的 Tunnel');
        }
        const saved = await BindTunnel(targetTunnel);
        setConfig(saved);
        setSettings(saved);
        setCredentials((current) => ({...current, tunnelToken: saved.tunnelToken || ''}));
        const result = await PullRoutesFromCloudflare();
        setConfig(result.config);
        setSettings(result.config);
        setCredentials((current) => ({...current, tunnelToken: result.config.tunnelToken || ''}));
        setSelectedRouteTunnelId(result.config.tunnelId || targetTunnel.id);
    }

    async function handleStart() {
        await withBusy('start', async () => {
            setStatus(await StartTunnel());
            setMessage('cloudflared 已启动');
        });
    }

    async function handleStop() {
        await withBusy('stop', async () => {
            setStatus(await StopTunnel());
            setMessage('cloudflared 正在停止');
        });
    }

    async function handleRestart() {
        await withBusy('restart', async () => {
            setStatus(await RestartTunnel());
            setMessage('cloudflared 已重启');
        });
    }

    async function handleRefreshCloudflare() {
        await withBusy('refresh', async () => {
            setStatus(await RefreshStatus());
            await refreshLocalState();
            setMessage('状态已刷新');
        });
    }

    async function withBusy(name: string, task: () => Promise<void>) {
        setBusy(name);
        setMessage('');
        try {
            await task();
        } catch (error) {
            setMessage(error instanceof Error ? error.message : String(error));
        } finally {
            setBusy('');
        }
    }

    return (
        <main className="app-shell">
            <aside className="sidebar">
                <div className="brand">
                    <Cloud size={28} strokeWidth={1.8}/>
                    <div>
                        <strong>Cloudflare Tunnel</strong>
                        <span>Desktop Manager</span>
                    </div>
                </div>

                <section className="status-block">
                    <div className={`live-dot ${status.running ? 'is-running' : ''}`}/>
                    <div>
                        <span className="muted">本地连接</span>
                        <strong>{status.running ? '运行中' : '已停止'}</strong>
                    </div>
                </section>

                <div className="metric-grid">
                    <Metric label="Tunnel" value={status.tunnelStatus || '未刷新'}/>
                    <Metric label="协议" value={settings.protocol || 'auto'}/>
                    <Metric label="映射" value={`${status.routeCount} 条`}/>
                    <Metric label="运行" value={uptimeText}/>
                </div>

                <div className="control-grid">
                    <button className="command primary" onClick={handleStart} disabled={status.running || busy !== ''} title="启动">
                        <Play size={16}/>启动
                    </button>
                    <button className="command" onClick={handleStop} disabled={!status.running || busy !== ''} title="停止">
                        <CircleStop size={16}/>停止
                    </button>
                    <button className="command" onClick={handleRestart} disabled={busy !== ''} title="重启">
                        <RotateCw size={16}/>重启
                    </button>
                    <button className="command" onClick={handleRefreshCloudflare} disabled={busy !== ''} title="刷新状态">
                        <RefreshCw size={16}/>刷新
                    </button>
                </div>

                <div className="sidebar-note">
                    <ShieldCheck size={16}/>
                    <span>Token 明文保存在本地配置文件，便于查看和复制。</span>
                </div>
            </aside>

            <section className="workspace">
                <header className="topbar">
                    <div>
                        <p className="eyebrow">macOS local connector</p>
                        <h1>{config.tunnelName || '未命名 Tunnel'}</h1>
                    </div>
                    <div className="topbar-meta">
                        <span>{status.cloudflaredVersion}</span>
                        <span>{status.pid ? `PID ${status.pid}` : '无进程'}</span>
                    </div>
                </header>

                {message && <div className={message.includes('失败') || message.includes('错误') || message.includes('请先') ? 'notice error' : 'notice'}>{message}</div>}

                <nav className="tabs" aria-label="工作区">
                    <button className={activePanel === 'config' ? 'active' : ''} onClick={() => setActivePanel('config')}>
                        <KeyRound size={16}/>基础配置
                    </button>
                    <button className={activePanel === 'tunnels' ? 'active' : ''} onClick={() => setActivePanel('tunnels')}>
                        <Cloud size={16}/>Tunnel 管理
                    </button>
                    <button className={activePanel === 'routes' ? 'active' : ''} onClick={() => setActivePanel('routes')}>
                        <Server size={16}/>域名映射
                    </button>
                    <button className={activePanel === 'logs' ? 'active' : ''} onClick={() => setActivePanel('logs')}>
                        <Activity size={16}/>日志
                    </button>
                </nav>

                {activePanel === 'config' && (
                    <div className="panel-grid">
                        <section className="panel">
                            <PanelTitle icon={<Cloud size={18}/>} title="Cloudflare 配置"/>
                            <div className={`install-strip ${installStatus.installed ? 'is-ok' : installStatus.installing ? 'is-installing' : 'is-warning'}`}>
                                <div className="install-main">
                                    {!installStatus.installed && <AlertTriangle size={18}/>}
                                    <div>
                                        <strong>{installStatus.installed ? 'cloudflared 已安装' : installStatus.installing ? 'cloudflared 正在安装' : 'cloudflared 未安装'}</strong>
                                        <span>{installStatus.installed ? installStatus.version : installStatus.installing ? '安装完成后会自动检测并刷新状态' : '需要安装后才能启动本地连接'}</span>
                                    </div>
                                </div>
                                {installStatus.installed ? (
                                    <span className="badge current">已安装</span>
                                ) : (
                                    <button className="command danger" type="button" disabled={busy !== '' || installStatus.installing} onClick={handleInstallCloudflared}>
                                        <RefreshCw className={installStatus.installing ? 'spin' : ''} size={16}/>{installStatus.installing ? '正在安装...' : '安装 cloudflared'}
                                    </button>
                                )}
                            </div>
                            <form className="form-grid" onSubmit={handleSaveSettings}>
                                <TextInput label="Account ID" helpLabel="获取 Account ID" helpURL={helpLinks.accountAndZone} value={settings.accountId} onChange={(value) => setSettings({...settings, accountId: value})}/>
                                <TextInput label="Zone ID" helpLabel="获取 Zone ID" helpURL={helpLinks.accountAndZone} value={settings.zoneId} onChange={(value) => setSettings({...settings, zoneId: value})}/>
                                <TextInput label="根域名" helpLabel="查看域名概览" helpURL={helpLinks.accountAndZone} value={settings.rootDomain} placeholder="example.com" onChange={(value) => setSettings({...settings, rootDomain: value})}/>
                                <div className="zone-picker">
                                    <button className="command" disabled={busy !== ''} type="button" onClick={handleFetchZones}>
                                        <RefreshCw size={16}/>获取根域名
                                    </button>
                                    {availableZones.length > 0 && (
                                        <label className="field">
                                            <span>选择根域名</span>
                                            <select value={settings.zoneId} onChange={(event) => {
                                                const zone = availableZones.find((item) => item.id === event.target.value);
                                                if (zone) {
                                                    void withBusy('selectZone', () => applyZone(zone));
                                                }
                                            }}>
                                                <option value="">请选择</option>
                                                {availableZones.map((zone) => (
                                                    <option value={zone.id} key={zone.id}>
                                                        {zone.name}{zone.account?.name ? ` / ${zone.account.name}` : ''}
                                                    </option>
                                                ))}
                                            </select>
                                        </label>
                                    )}
                                </div>
                                <div className="field">
                                    <span className="field-header">
                                        <span>传输协议</span>
                                        <HelpLink label="协议说明" url={helpLinks.protocol}/>
                                    </span>
                                    <label className="sr-only" htmlFor="transport-protocol">传输协议</label>
                                    <select id="transport-protocol" value={settings.protocol} onChange={(event) => setSettings({...settings, protocol: event.target.value})}>
                                        <option value="auto">auto</option>
                                        <option value="quic">quic</option>
                                        <option value="http2">http2</option>
                                    </select>
                                </div>
                                <label className="toggle-row">
                                    <input type="checkbox" checked={settings.autoRestart} onChange={(event) => setSettings({...settings, autoRestart: event.target.checked})}/>
                                    <span>网络变化或进程异常退出后自动重启</span>
                                </label>
                                <div className="form-actions">
                                    <button className="command primary" disabled={busy !== ''} type="submit">
                                        <Save size={16}/>保存配置
                                    </button>
                                </div>
                                <div className="inline-help">
                                    <HelpLink label="获取 Account ID / Zone ID" url={helpLinks.accountAndZone}/>
                                    <HelpLink label="打开 API Token 页面" url={helpLinks.apiToken}/>
                                </div>
                            </form>
                        </section>

                        <section className="panel">
                            <PanelTitle icon={<KeyRound size={18}/>} title="本地凭据"/>
                            <form className="form-grid single" onSubmit={handleCredentials}>
                                <label className="field">
                                    <span>认证方式</span>
                                    <select value={credentials.authType} onChange={(event) => setCredentials({...credentials, authType: event.target.value})}>
                                        <option value="api_token">API Token</option>
                                        <option value="global_key">Global API Key</option>
                                    </select>
                                </label>
                                {credentials.authType === 'global_key' && (
                                    <TextInput label="Cloudflare 邮箱" value={credentials.authEmail} onChange={(value) => setCredentials({...credentials, authEmail: value})}/>
                                )}
                                <TextInput
                                    label={credentials.authType === 'global_key' ? 'Global API Key' : 'Cloudflare API Token'}
                                    helpLabel={credentials.authType === 'global_key' ? '打开 API Keys 页面' : '创建 API Token'}
                                    helpURL={helpLinks.apiToken}
                                    value={credentials.apiToken}
                                    onChange={(value) => setCredentials({...credentials, apiToken: value})}
                                />
                                <div className="credential-state">
                                    <span className={status.apiTokenSet ? 'ok' : ''}>{status.authType === 'global_key' ? 'Global API Key' : 'API Token'} {status.apiTokenSet ? '已保存' : '未保存'}</span>
                                </div>
                                <button className="command primary" disabled={busy !== ''} type="submit">
                                    <KeyRound size={16}/>保存凭据
                                </button>
                                <div className="inline-help">
                                    <HelpLink label="打开 API Token 页面" url={helpLinks.apiToken}/>
                                </div>
                            </form>
                        </section>
                    </div>
                )}

                {activePanel === 'tunnels' && (
                    <section className="panel">
                        <div className="panel-heading with-action">
                            <PanelTitle icon={<Cloud size={18}/>} title="Tunnel 管理"/>
                            <div className="route-actions">
                                <button className="command" type="button" disabled={busy !== ''} onClick={() => handleListTunnels(true)}>
                                    <RefreshCw size={16}/>刷新
                                </button>
                                <button className="command primary" type="button" disabled={busy !== ''} onClick={handleCreateTunnel}>
                                    <Plus size={16}/>新增 Tunnel
                                </button>
                            </div>
                        </div>
                        <div className="tunnel-create-row">
                            <TextInput label="新增 Tunnel 名称" helpLabel="创建 Tunnel" helpURL={helpLinks.tunnelSetup} value={settings.tunnelName} placeholder="desktop-tunnel" onChange={(value) => setSettings({...settings, tunnelName: value})}/>
                        </div>
                        <div className="table-shell">
                            <table className="data-table">
                                <thead>
                                    <tr>
                                        <th>名称</th>
                                        <th>Tunnel ID</th>
                                        <th>状态</th>
                                        <th>当前状态</th>
                                        <th>操作</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {availableTunnels.length === 0 && (
                                        <tr>
                                            <td colSpan={5} className="table-empty">{busy === 'listTunnels' ? '正在读取 Tunnel 列表' : '暂无 Tunnel，请点击刷新'}</td>
                                        </tr>
                                    )}
                                    {availableTunnels.map((tunnel) => (
                                        <tr key={tunnel.id}>
                                            <td>
                                                <strong>{tunnel.name || '未命名 Tunnel'}</strong>
                                            </td>
                                            <td className="mono-cell">{tunnel.id}</td>
                                            <td><span className="badge">{tunnel.status || 'unknown'}</span></td>
                                            <td>{config.tunnelId === tunnel.id ? <span className="badge current">当前</span> : <span className="muted">未选中</span>}</td>
                                            <td>
                                                {deleteTunnelId === tunnel.id ? (
                                                    <div className="delete-confirm">
                                                        <label className="checkbox-line">
                                                            <input type="checkbox" checked={deleteTunnelDNS} onChange={(event) => setDeleteTunnelDNS(event.target.checked)}/>
                                                            <span>同时删除 DNS</span>
                                                        </label>
                                                        <div className="row-actions confirm-actions">
                                                            <button className="command danger compact" type="button" disabled={busy !== ''} onClick={() => handleDeleteTunnel(tunnel)}>
                                                                确认
                                                            </button>
                                                            <button className="command compact" type="button" disabled={busy !== ''} onClick={() => {
                                                                setDeleteTunnelId('');
                                                                setDeleteTunnelDNS(false);
                                                                setMessage('已取消删除 Tunnel');
                                                            }}>
                                                                取消
                                                            </button>
                                                        </div>
                                                    </div>
                                                ) : (
                                                    <div className="row-actions">
                                                        {config.tunnelId !== tunnel.id && (
                                                            <button className="command compact" type="button" disabled={busy !== ''} onClick={() => void withBusy(`bindTunnel-${tunnel.id}`, () => applyTunnel(tunnel))}>
                                                                设为当前
                                                            </button>
                                                        )}
                                                        <button className="icon-button danger" type="button" title="删除 Tunnel" disabled={busy !== ''} onClick={() => {
                                                            setDeleteTunnelId(tunnel.id);
                                                            setDeleteTunnelDNS(false);
                                                            setMessage(`请再次确认删除 Tunnel: ${tunnel.name || tunnel.id}`);
                                                        }}>
                                                            <Trash2 size={15}/>
                                                        </button>
                                                    </div>
                                                )}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                        <div className="form-grid single tunnel-token-form">
                            <TextInput label="Tunnel Token" helpLabel="获取 Tunnel Token" helpURL={helpLinks.tunnelToken} value={credentials.tunnelToken} onChange={() => {}} readOnly/>
                            <div className="credential-state">
                                <span className={status.tunnelTokenSet ? 'ok' : ''}>Tunnel Token {status.tunnelTokenSet ? '已从 Cloudflare 获取并保存到本地' : '切换或启动 Tunnel 时自动从 Cloudflare 获取'}</span>
                            </div>
                        </div>
                    </section>
                )}

                {activePanel === 'routes' && (
                    <div className="panel-grid routes-layout">
                        <section className="panel">
                            <PanelTitle icon={<Plus size={18}/>} title={routeForm.id ? '编辑映射' : '新增映射'}/>
                            <form className="form-grid single" onSubmit={handleRouteSubmit}>
                                <label className="field">
                                    <span>Tunnel</span>
                                    <select value={selectedRouteTunnelId || config.tunnelId} onChange={(event) => handleRouteTunnelChange(event.target.value)}>
                                        <option value={config.tunnelId}>{config.tunnelName || config.tunnelId || '当前 Tunnel'}</option>
                                        {availableTunnels.filter((tunnel) => tunnel.id !== config.tunnelId).map((tunnel) => (
                                            <option value={tunnel.id} key={tunnel.id}>{tunnel.name || tunnel.id}</option>
                                        ))}
                                    </select>
                                </label>
                                <TextInput label="公开域名" value={routeForm.hostname} placeholder="app.example.com" onChange={(value) => setRouteForm({...routeForm, hostname: value})}/>
                                <label className="field">
                                    <span>本地协议</span>
                                    <select value={routeForm.serviceProtocol} onChange={(event) => setRouteForm({...routeForm, serviceProtocol: event.target.value})}>
                                        <option value="http">http</option>
                                        <option value="https">https</option>
                                    </select>
                                </label>
                                <TextInput label="本地主机" value={routeForm.serviceHost} placeholder="localhost" onChange={(value) => setRouteForm({...routeForm, serviceHost: value})}/>
                                <label className="field">
                                    <span>端口</span>
                                    <input type="number" min={1} max={65535} value={routeForm.servicePort} placeholder="端口" onChange={(event) => setRouteForm({...routeForm, servicePort: event.target.value === '' ? '' : Number(event.target.value)})}/>
                                </label>
                                <label className="toggle-row">
                                    <input type="checkbox" checked={routeForm.enabled} onChange={(event) => setRouteForm({...routeForm, enabled: event.target.checked})}/>
                                    <span>启用这条映射</span>
                                </label>
                                <div className="form-actions">
                                    <button className="command primary" disabled={busy !== ''} type="submit">
                                        <Save size={16}/>{routeForm.id ? '更新并同步' : '添加并同步'}
                                    </button>
                                    {routeForm.id && (
                                        <button className="command" type="button" onClick={() => setRouteForm(blankRoute)}>
                                            取消
                                        </button>
                                    )}
                                </div>
                            </form>
                        </section>

                        <section className="panel wide">
                            <PanelTitle icon={<Server size={18}/>} title="域名映射"/>
                            <div className="table-shell">
                                <table className="data-table">
                                    <thead>
                                        <tr>
                                            <th>公开域名</th>
                                            <th>本地服务</th>
                                            <th>状态</th>
                                            <th>操作</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {config.routes.length === 0 && (
                                            <tr>
                                                <td colSpan={4} className="table-empty">暂无映射</td>
                                            </tr>
                                        )}
                                        {config.routes.map((route) => (
                                            <tr key={route.id}>
                                                <td><strong>{route.hostname}</strong></td>
                                                <td>{route.serviceProtocol}://{route.serviceHost}:{route.servicePort}</td>
                                                <td><span className={route.enabled ? 'badge ok' : 'badge'}>{route.enabled ? '启用' : '停用'}</span></td>
                                                <td>
                                                    <div className="row-actions">
                                                        <button className="icon-button" title="编辑" onClick={() => setRouteForm(route)}>
                                                            <Pencil size={15}/>
                                                        </button>
                                                        <button className="icon-button danger" title="删除" onClick={() => handleRemoveRoute(route)} disabled={busy !== ''}>
                                                            <Trash2 size={15}/>
                                                        </button>
                                                    </div>
                                                </td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                        </section>
                    </div>
                )}

                {activePanel === 'logs' && (
                    <section className="panel">
                        <PanelTitle icon={<Activity size={18}/>} title="运行日志"/>
                        <div className="log-list">
                            {logs.length === 0 && <div className="empty">暂无日志</div>}
                            {logs.slice().reverse().map((entry, index) => (
                                <div className={`log-row ${entry.level} ${entry.category}`} key={`${entry.time}-${index}`}>
                                    <time>{formatTime(entry.time)}</time>
                                    <span>{entry.source}</span>
                                    <strong>{entry.category}</strong>
                                    <p>{entry.message}</p>
                                </div>
                            ))}
                        </div>
                    </section>
                )}

            </section>
        </main>
    );
}

function Metric({label, value}: { label: string; value: string }) {
    return (
        <div className="metric">
            <span>{label}</span>
            <strong>{value}</strong>
        </div>
    );
}

function PanelTitle({icon, title}: { icon: JSX.Element; title: string }) {
    return (
        <div className="panel-title">
            {icon}
            <h2>{title}</h2>
        </div>
    );
}

function TextInput({label, value, onChange, placeholder = '', type = 'text', helpLabel = '', helpURL = '', readOnly = false}: {
    label: string;
    value: string;
    onChange: (value: string) => void;
    placeholder?: string;
    type?: string;
    helpLabel?: string;
    helpURL?: string;
    readOnly?: boolean;
}) {
    return (
        <div className="field">
            <span className="field-header">
                <span>{label}</span>
                <span className="field-actions">
                    {value && <CopyButton value={value} label={`复制 ${label}`}/>}
                    {helpLabel && helpURL && <HelpLink label={helpLabel} url={helpURL}/>}
                </span>
            </span>
            <input aria-label={label} type={type} value={value} placeholder={placeholder} readOnly={readOnly} onChange={(event) => onChange(event.target.value)}/>
        </div>
    );
}

function CopyButton({value, label}: { value: string; label: string }) {
    async function copyValue() {
        try {
            await navigator.clipboard.writeText(value);
        } catch {
            const input = document.createElement('textarea');
            input.value = value;
            input.style.position = 'fixed';
            input.style.opacity = '0';
            document.body.appendChild(input);
            input.select();
            document.execCommand('copy');
            document.body.removeChild(input);
        }
    }

    return (
        <button className="help-link" type="button" onClick={copyValue} title={label}>
            <Copy size={13}/>
            复制
        </button>
    );
}

function HelpLink({label, url}: { label: string; url: string }) {
    return (
        <button className="help-link" type="button" onClick={() => BrowserOpenURL(url)} title={label}>
            <ExternalLink size={13}/>
            {label}
        </button>
    );
}

function formatDuration(seconds: number) {
    if (!seconds) {
        return '0s';
    }
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const rest = seconds % 60;
    if (hours > 0) {
        return `${hours}h ${minutes}m`;
    }
    if (minutes > 0) {
        return `${minutes}m ${rest}s`;
    }
    return `${rest}s`;
}

function formatTime(value: string) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
        return '--:--:--';
    }
    return date.toLocaleTimeString();
}

export default App;
