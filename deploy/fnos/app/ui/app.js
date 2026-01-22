const TOKEN_KEY = 'clearvault_access_token';

// 全局 Fetch 封装
async function authenticatedFetch(url, options = {}) {
    options.headers = options.headers || {};
    const token = localStorage.getItem(TOKEN_KEY);
    if (token) {
        options.headers['Authorization'] = `Bearer ${token}`;
    }

    const res = await originalFetch(url, options);
    
    if (res.status === 401) {
        showLoginModal();
        throw new Error('Unauthorized');
    }
    
    return res;
}

// 替换原始 fetch 调用
const originalFetch = window.fetch;
window.fetch = authenticatedFetch;

function setHidden(el, hidden) {
    if (!el) {
        return;
    }
    if (hidden) {
        el.classList.add('hidden');
    } else {
        el.classList.remove('hidden');
    }
}

document.addEventListener('DOMContentLoaded', async () => {
    // 绑定登录按钮
    document.getElementById('login-btn').addEventListener('click', handleLogin);
    
    document.getElementById('setup-form').addEventListener('submit', handleSetupSubmit);
    document.getElementById('setup-manual-key-check').addEventListener('change', toggleSetupManualKey);
    document.getElementById('setup-remote-type').addEventListener('change', toggleSetupRemoteFields);

    document.getElementById('config-form').addEventListener('submit', handleConfigSave);
    document.getElementById('remote-type').addEventListener('change', toggleRemoteFields);
    document.getElementById('mount-btn').addEventListener('click', handleMount);
    document.getElementById('unmount-btn').addEventListener('click', handleUnmount);
    document.getElementById('encrypt-run-btn').addEventListener('click', handleToolEncrypt);
    document.getElementById('export-run-btn').addEventListener('click', handleToolExport);
    document.getElementById('import-run-btn').addEventListener('click', handleToolImport);

    toggleRemoteFields();
    toggleSetupRemoteFields();

    await bootstrap();

    const modalCloseBtn = document.getElementById('modal-close-btn');
    if (modalCloseBtn) {
        modalCloseBtn.addEventListener('click', () => {
            document.getElementById('security-modal').classList.add('hidden');
        });
    }

    const confirmCancelBtn = document.getElementById('confirm-cancel-btn');
    if (confirmCancelBtn) {
        confirmCancelBtn.addEventListener('click', () => {
            pendingConfig = null;
            pendingIsInitialSetup = false;
            pendingNewMasterKey = "";
            document.getElementById('confirm-modal').classList.add('hidden');
            populateConfigForm();
            const statusEl = document.getElementById('save-status');
            statusEl.textContent = "已取消修改";
            statusEl.className = "mt-2 text-sm text-gray-600";
        });
    }
});

async function bootstrap() {
    const status = await fetchStatus();
    if (!status) {
        return
    }

    if (status.initialized === false) {
        showSetupModal();
        await fetchPaths();
        await fetchConfig();
        return
    }

    await fetchPaths();
    await fetchConfig();
    fetchMountStatus();
}

// Login Logic
function showLoginModal() {
    document.getElementById('login-modal').classList.remove('hidden');
    document.getElementById('login-token').value = '';
    document.getElementById('login-error').classList.add('hidden');
}

async function handleLogin() {
    const token = document.getElementById('login-token').value.trim();
    if (!token) {
        const errEl = document.getElementById('login-error');
        errEl.textContent = "请输入访问密码";
        errEl.classList.remove('hidden');
        return;
    }
    
    // 尝试验证（调用 status 接口，手动构造 header）
    try {
        const res = await originalFetch('/api/v1/status', {
            headers: { 'Authorization': `Bearer ${token}` }
        });
        
        if (res.ok) {
            localStorage.setItem(TOKEN_KEY, token);
            document.getElementById('login-modal').classList.add('hidden');
            await bootstrap();
        } else {
            const errEl = document.getElementById('login-error');
            errEl.textContent = "密码错误，请重试";
            errEl.classList.remove('hidden');
        }
    } catch (e) {
        console.error("Login check failed", e);
        const errEl = document.getElementById('login-error');
        errEl.textContent = "网络错误";
        errEl.classList.remove('hidden');
    }
}

async function fetchPaths() {
    try {
        const res = await fetch('/api/v1/paths');
        const data = await res.json();
        const select = document.getElementById('local-path-select');
        const setupSelect = document.getElementById('setup-local-path-select');
        const mountSelect = document.getElementById('mountpoint-select');
        const encryptInBase = document.getElementById('encrypt-in-base');
        const encryptOutBase = document.getElementById('encrypt-out-base');
        const exportOutBase = document.getElementById('export-out-base');
        const importInBase = document.getElementById('import-in-base');
        
        const rawPaths = (data.paths && data.paths.length > 0) ? data.paths : [];
        const paths = rawPaths.map(p => (p || '').trim()).filter(p => p !== '');
        resetSelectOptions(select, paths);
        resetSelectOptions(setupSelect, paths);
        resetSelectOptions(mountSelect, paths);
        resetSelectOptions(encryptInBase, paths);
        resetSelectOptions(encryptOutBase, paths);
        resetSelectOptions(exportOutBase, paths);
        resetSelectOptions(importInBase, paths);
    } catch (e) {
        console.error("Failed to fetch paths", e);
    }
}

function resetSelectOptions(selectEl, paths) {
    if (!selectEl) {
        return;
    }
    while (selectEl.options.length > 1) {
        selectEl.remove(1);
    }
    paths.forEach(p => {
        const opt = document.createElement('option');
        opt.value = p;
        opt.textContent = p;
        selectEl.appendChild(opt);
    });
}

function joinPath(base, rel) {
    const b = (base || '').trim().replace(/\/+$/, '');
    const r = (rel || '').trim().replace(/^\/+/, '');
    if (!b) {
        return '';
    }
    if (!r) {
        return b;
    }
    return `${b}/${r}`;
}

function setStatus(el, text, ok) {
    if (!el) {
        return;
    }
    el.textContent = text;
    if (ok === true) {
        el.className = 'mt-2 text-sm text-green-600';
    } else if (ok === false) {
        el.className = 'mt-2 text-sm text-red-600';
    } else {
        el.className = 'mt-2 text-sm text-gray-600';
    }
}

async function fetchMountStatus() {
    try {
        const res = await fetch('/api/v1/mount/status');
        const data = await res.json();
        const statusEl = document.getElementById('mount-status');
        const mountBtn = document.getElementById('mount-btn');
        const unmountBtn = document.getElementById('unmount-btn');

        if (data.mounted) {
            statusEl.textContent = `已挂载：${data.mountpoint}`;
            statusEl.className = 'text-sm text-green-700';
            mountBtn.classList.add('hidden');
            unmountBtn.classList.remove('hidden');
        } else {
            statusEl.textContent = '未挂载';
            statusEl.className = 'text-sm text-gray-600';
            mountBtn.classList.remove('hidden');
            unmountBtn.classList.add('hidden');
        }
    } catch (e) {
        const statusEl = document.getElementById('mount-status');
        statusEl.textContent = '无法获取挂载状态';
        statusEl.className = 'text-sm text-red-600';
    }
}

async function handleMount(e) {
    e.preventDefault();
    const statusEl = document.getElementById('mount-status');
    statusEl.textContent = '正在挂载…';
    statusEl.className = 'text-sm text-gray-600';

    const mountpoint = document.getElementById('mountpoint-select').value;
    if (!mountpoint) {
        statusEl.textContent = '请选择一个已授权的挂载点。';
        statusEl.className = 'text-sm text-red-600';
        return;
    }

    try {
        const res = await fetch('/api/v1/mount', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ mountpoint })
        });
        if (!res.ok) {
            const text = await res.text();
            statusEl.textContent = `挂载失败：${text}`;
            statusEl.className = 'text-sm text-red-600';
            return;
        }
        await fetchMountStatus();
    } catch (e2) {
        statusEl.textContent = `挂载失败：${e2.message}`;
        statusEl.className = 'text-sm text-red-600';
    }
}

async function handleUnmount(e) {
    e.preventDefault();
    const statusEl = document.getElementById('mount-status');
    statusEl.textContent = '正在卸载…';
    statusEl.className = 'text-sm text-gray-600';
    try {
        const res = await fetch('/api/v1/unmount', { method: 'POST' });
        if (!res.ok) {
            const text = await res.text();
            statusEl.textContent = `卸载失败：${text}`;
            statusEl.className = 'text-sm text-red-600';
            return;
        }
        await fetchMountStatus();
    } catch (e2) {
        statusEl.textContent = `卸载失败：${e2.message}`;
        statusEl.className = 'text-sm text-red-600';
    }
}

function toggleRemoteFields() {
    const remoteTypeEl = document.getElementById('remote-type');
    if (!remoteTypeEl) {
        return;
    }
    const type = remoteTypeEl.value;
    const urlContainer = document.getElementById('url-container');
    const localPathContainer = document.getElementById('local-path-container');
    const urlLabel = document.getElementById('url-label');
    const userLabel = document.getElementById('user-label');
    const passLabel = document.getElementById('pass-label');
    const remoteUser = document.getElementById('remote-user');
    const remotePass = document.getElementById('remote-pass');
    const regionContainer = document.getElementById('region-container');
    const bucketContainer = document.getElementById('bucket-container');

    if (type === 'local') {
        setHidden(urlContainer, true);
        setHidden(localPathContainer, false);
        setHidden(regionContainer, true);
        setHidden(bucketContainer, true);
        if (remoteUser) {
            remoteUser.disabled = true;
        }
        if (remotePass) {
            remotePass.disabled = true;
        }
    } else {
        setHidden(urlContainer, false);
        setHidden(localPathContainer, true);
        if (remoteUser) {
            remoteUser.disabled = false;
        }
        if (remotePass) {
            remotePass.disabled = false;
        }
        
        if (type === 's3') {
            if (urlLabel) {
                urlLabel.textContent = 'Endpoint';
            }
            if (userLabel) {
                userLabel.textContent = 'Access Key';
            }
            if (passLabel) {
                passLabel.textContent = 'Secret Key';
            }
            setHidden(regionContainer, false);
            setHidden(bucketContainer, false);
        } else {
            if (urlLabel) {
                urlLabel.textContent = 'WebDAV URL';
            }
            if (userLabel) {
                userLabel.textContent = '用户名';
            }
            if (passLabel) {
                passLabel.textContent = '密码';
            }
            setHidden(regionContainer, true);
            setHidden(bucketContainer, true);
        }
    }
}

async function fetchStatus() {
    try {
        const res = await fetch('/api/v1/status');
        const data = await res.json();
        
        document.getElementById('version').textContent = data.version || '-';
        document.getElementById('uptime').textContent = data.uptime;
        
        const badge = document.getElementById('status-badge');
        badge.textContent = data.status === 'running' ? '运行中' : '异常';
        if (data.status === 'running') {
            badge.className = "px-3 py-1 rounded-full text-sm font-semibold bg-green-100 text-green-800";
        } else {
            badge.className = "px-3 py-1 rounded-full text-sm font-semibold bg-red-100 text-red-800";
        }
        return data
    } catch (e) {
        if (e && e.message !== 'Unauthorized') {
            console.error("Failed to fetch status", e);
        }
        return null
    }
}

// Setup Logic
function showSetupModal() {
    document.getElementById('setup-modal').classList.remove('hidden');
    // Pre-fill setup form with current defaults or paths
    // Wait for fetchPaths to complete if needed, but it runs in parallel
    // We can retry setting select options after a short delay or ensure fetchPaths is done
    // Since fetchPaths is awaited in DOMContentLoaded before setup check? No, parallel.
    // Let's just populate select options when setup modal opens if paths are ready.
    // Actually paths are loaded into all selects by ID, so setup-local-path-select should be populated.
    toggleSetupRemoteFields();
}

function toggleSetupManualKey() {
    const checked = document.getElementById('setup-manual-key-check').checked;
    const container = document.getElementById('setup-manual-key-container');
    if (checked) {
        container.classList.remove('hidden');
    } else {
        container.classList.add('hidden');
        document.getElementById('setup-master-key').value = '';
    }
}

function toggleSetupRemoteFields() {
    const setupRemoteTypeEl = document.getElementById('setup-remote-type');
    if (!setupRemoteTypeEl) {
        return;
    }
    const type = setupRemoteTypeEl.value;
    const urlContainer = document.getElementById('setup-url-container');
    const localPathContainer = document.getElementById('setup-local-path-container');
    const regionContainer = document.getElementById('setup-region-container');
    const bucketContainer = document.getElementById('setup-bucket-container');
    const urlLabel = document.getElementById('setup-url-label');
    
    const setupRemoteUserEl = document.getElementById('setup-remote-user');
    const setupRemotePassEl = document.getElementById('setup-remote-pass');
    const userLabel = setupRemoteUserEl ? setupRemoteUserEl.previousElementSibling : null;
    const passLabel = setupRemotePassEl ? setupRemotePassEl.previousElementSibling : null;

    if (type === 'local') {
        setHidden(urlContainer, true);
        setHidden(localPathContainer, false);
        setHidden(regionContainer, true);
        setHidden(bucketContainer, true);
        if (setupRemoteUserEl) {
            setupRemoteUserEl.disabled = true;
        }
        if (setupRemotePassEl) {
            setupRemotePassEl.disabled = true;
        }
    } else {
        setHidden(urlContainer, false);
        setHidden(localPathContainer, true);
        if (setupRemoteUserEl) {
            setupRemoteUserEl.disabled = false;
        }
        if (setupRemotePassEl) {
            setupRemotePassEl.disabled = false;
        }
        
        if (type === 's3') {
            if (urlLabel) {
                urlLabel.textContent = 'Endpoint';
            }
            if (userLabel) {
                userLabel.textContent = 'Access Key';
            }
            if (passLabel) {
                passLabel.textContent = 'Secret Key';
            }
            setHidden(regionContainer, false);
            setHidden(bucketContainer, false);
        } else {
            if (urlLabel) {
                urlLabel.textContent = 'WebDAV URL';
            }
            if (userLabel) {
                userLabel.textContent = '用户名';
            }
            if (passLabel) {
                passLabel.textContent = '密码';
            }
            setHidden(regionContainer, true);
            setHidden(bucketContainer, true);
        }
    }
}

async function handleSetupSubmit(e) {
    e.preventDefault();
    const statusEl = document.getElementById('setup-status');
    statusEl.textContent = "正在初始化…";
    statusEl.className = "text-center text-gray-600 mt-2";

    const type = document.getElementById('setup-remote-type').value;
    const configData = {
        remote: { type },
        security: {}
    };

    if (type === 's3') {
        configData.remote.endpoint = document.getElementById('setup-remote-url').value;
        configData.remote.access_key = document.getElementById('setup-remote-user').value;
        configData.remote.secret_key = document.getElementById('setup-remote-pass').value;
        configData.remote.region = document.getElementById('setup-remote-region').value;
        configData.remote.bucket = document.getElementById('setup-remote-bucket').value;
    } else if (type === 'local') {
        configData.remote.local_path = document.getElementById('setup-local-path-select').value;
        if (!configData.remote.local_path) {
            statusEl.textContent = "请选择本地路径";
            statusEl.className = "text-center text-red-600 mt-2";
            return;
        }
    } else {
        configData.remote.url = document.getElementById('setup-remote-url').value;
        configData.remote.user = document.getElementById('setup-remote-user').value;
        configData.remote.pass = document.getElementById('setup-remote-pass').value;
    }

    const manualKey = document.getElementById('setup-master-key').value;
    if (document.getElementById('setup-manual-key-check').checked && manualKey) {
        configData.security.master_key = manualKey;
    } else {
        // Leave empty to trigger auto-generation
        configData.security.master_key = "";
    }

    // Merge with existing config (loaded via fetchConfig) to preserve other settings like listen port
    const finalConfig = JSON.parse(JSON.stringify(currentConfig || {}));
    finalConfig.remote = { ...(finalConfig.remote || {}), ...configData.remote };
    finalConfig.security = { ...(finalConfig.security || {}), ...configData.security };

    try {
        const res = await fetch('/api/v1/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(finalConfig)
        });

        if (res.ok) {
            const respData = await res.json();
            document.getElementById('setup-modal').classList.add('hidden');
            
            // Show master key backup modal if generated
            if (!manualKey && respData.master_key) {
                document.getElementById('modal-master-key').textContent = respData.master_key;
                document.getElementById('security-modal').classList.remove('hidden');
                
                // Reload page after modal close
                document.getElementById('modal-close-btn').onclick = () => {
                    location.reload();
                };
            } else {
                location.reload();
            }
        } else {
            const text = await res.text();
            statusEl.textContent = "初始化失败：" + text;
            statusEl.className = "text-center text-red-600 mt-2";
        }
    } catch (e) {
        statusEl.textContent = "网络错误：" + e.message;
        statusEl.className = "text-center text-red-600 mt-2";
    }
}

let currentConfig = {};

function populateConfigForm() {
    currentConfig = currentConfig || {};
    currentConfig.server = currentConfig.server || {};
    currentConfig.remote = currentConfig.remote || {};
    currentConfig.security = currentConfig.security || {};
    currentConfig.storage = currentConfig.storage || {};

    document.getElementById('remote-type').value = currentConfig.remote.type || 'webdav';
    toggleRemoteFields();

    if (currentConfig.remote.type === 'local') {
        const select = document.getElementById('local-path-select');
        select.value = currentConfig.remote.local_path || '';
    } else {
        document.getElementById('remote-url').value = currentConfig.remote.url || currentConfig.remote.endpoint || '';
        document.getElementById('remote-user').value = currentConfig.remote.user || currentConfig.remote.access_key || '';
        document.getElementById('remote-pass').value = currentConfig.remote.pass || currentConfig.remote.secret_key || '';
        document.getElementById('remote-region').value = currentConfig.remote.region || '';
        document.getElementById('remote-bucket').value = currentConfig.remote.bucket || '';
    }

    document.getElementById('master-key').value = currentConfig.security.master_key || '';
}

async function fetchConfig() {
    try {
        const res = await fetch('/api/v1/config');
        currentConfig = await res.json();
        populateConfigForm();

    } catch (e) {
        console.error("Failed to fetch config", e);
    }
}

let pendingConfig = null;
let pendingIsInitialSetup = false;
let pendingNewMasterKey = "";

function openConfirmModal(titleHtml, messageHtml, hintHtml, onConfirm) {
    const titleEl = document.getElementById('confirm-title');
    const messageEl = document.getElementById('confirm-message');
    const hintEl = document.getElementById('confirm-hint');
    if (titleEl) {
        titleEl.innerHTML = titleHtml;
    }
    if (messageEl) {
        messageEl.innerHTML = messageHtml;
    }
    if (hintEl) {
        hintEl.innerHTML = hintHtml;
    }

    const confirmBtn = document.getElementById('confirm-ok-btn');
    const newBtn = confirmBtn.cloneNode(true);
    confirmBtn.parentNode.replaceChild(newBtn, confirmBtn);

    newBtn.addEventListener('click', () => {
        document.getElementById('confirm-modal').classList.add('hidden');
        onConfirm();
    });

    document.getElementById('confirm-modal').classList.remove('hidden');
}

async function handleConfigSave(e) {
    e.preventDefault();
    const statusEl = document.getElementById('save-status');
    statusEl.textContent = "正在校验…";
    statusEl.className = "mt-2 text-sm text-gray-600";

    const type = document.getElementById('remote-type').value;
    
    // Check if this is initial setup (before updating currentConfig)
    const oldMasterKey = currentConfig.security && currentConfig.security.master_key;
    const isInitialSetup = !oldMasterKey || oldMasterKey === "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY";

    // Clone config to avoid modifying the global state before confirmation
    let newConfig = JSON.parse(JSON.stringify(currentConfig));
    newConfig.server = newConfig.server || {};
    newConfig.remote = newConfig.remote || {};
    newConfig.security = newConfig.security || {};
    newConfig.storage = newConfig.storage || {};
    
    // Update newConfig object
    newConfig.remote.type = type;
    
    if (type === 's3') {
        newConfig.remote.endpoint = document.getElementById('remote-url').value;
        newConfig.remote.access_key = document.getElementById('remote-user').value;
        newConfig.remote.secret_key = document.getElementById('remote-pass').value;
        newConfig.remote.region = document.getElementById('remote-region').value;
        newConfig.remote.bucket = document.getElementById('remote-bucket').value;
    } else if (type === 'local') {
        const localPath = document.getElementById('local-path-select').value;
        if (!localPath) {
            statusEl.textContent = "请选择一个已授权的本地路径。";
            statusEl.className = "mt-2 text-sm text-red-600";
            return;
        }
        newConfig.remote.local_path = localPath;
    } else {
        newConfig.remote.url = document.getElementById('remote-url').value;
        newConfig.remote.user = document.getElementById('remote-user').value;
        newConfig.remote.pass = document.getElementById('remote-pass').value;
    }
    
    const newMasterKey = document.getElementById('master-key').value;
    newConfig.security.master_key = newMasterKey;

    const oldRemote = (currentConfig && currentConfig.remote) ? currentConfig.remote : {};
    const oldRemoteType = oldRemote.type || "";
    const oldRemoteConfigured = !!(oldRemoteType || oldRemote.url || oldRemote.endpoint || oldRemote.local_path);
    let remoteDanger = false;
    let remoteDangerLabel = "";
    if (oldRemoteConfigured && type !== oldRemoteType) {
        remoteDanger = true;
        remoteDangerLabel = "远端存储类型";
    } else if (oldRemoteConfigured && type === oldRemoteType) {
        if (type === "s3") {
            const oldEndpoint = oldRemote.endpoint || "";
            const newEndpoint = newConfig.remote.endpoint || "";
            if (oldEndpoint && newEndpoint !== oldEndpoint) {
                remoteDanger = true;
                remoteDangerLabel = "远端存储地址";
            }
        } else if (type === "local") {
            const oldPath = oldRemote.local_path || "";
            const newPath = newConfig.remote.local_path || "";
            if (oldPath && newPath !== oldPath) {
                remoteDanger = true;
                remoteDangerLabel = "远端存储路径";
            }
        } else {
            const oldUrl = oldRemote.url || "";
            const newUrl = newConfig.remote.url || "";
            if (oldUrl && newUrl !== oldUrl) {
                remoteDanger = true;
                remoteDangerLabel = "远端存储地址";
            }
        }
    }

    const masterKeyChanged = !isInitialSetup && newMasterKey !== oldMasterKey;

    if (masterKeyChanged || remoteDanger) {
        pendingConfig = newConfig;
        pendingIsInitialSetup = isInitialSetup;
        pendingNewMasterKey = newMasterKey;

        let msg = "您正在进行以下高风险修改：<br>";
        let idx = 1;
        if (masterKeyChanged) {
            msg += `${idx}. 修改主密钥：通常会导致<strong>所有历史数据无法解密</strong>。<br>`;
            idx += 1;
        }
        if (remoteDanger) {
            msg += `${idx}. 修改${remoteDangerLabel}：可能导致历史数据不可访问或写入到新的位置。<br>`;
            idx += 1;
        }

        const hint = "<strong>强烈建议：</strong><br>1. 确认您理解变更后果，并已备份元数据。<br>2. 如非迁移/重建场景，请勿修改。";

        openConfirmModal(
            "⚠️ 极度危险操作确认",
            msg,
            hint,
            () => {
                const cfg = pendingConfig;
                const initial = pendingIsInitialSetup;
                const key = pendingNewMasterKey;
                pendingConfig = null;
                pendingIsInitialSetup = false;
                pendingNewMasterKey = "";
                performSave(cfg, initial, key);
            }
        );
        return;
    }

    // Normal save
    performSave(newConfig, isInitialSetup, newMasterKey);
}

async function performSave(configToSave, isInitialSetup, newMasterKey) {
    const statusEl = document.getElementById('save-status');
    statusEl.textContent = "正在保存…";

    try {
        const res = await fetch('/api/v1/config', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(configToSave)
        });
        
        if (res.ok) {
            // Update global config on success
            currentConfig = configToSave;
            
            statusEl.textContent = "保存成功！请重启应用使配置生效。";
            statusEl.className = "mt-2 text-sm text-green-600";
            
            // Show security warning modal if this was initial setup
            if (isInitialSetup && newMasterKey && newMasterKey !== "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY") {
                document.getElementById('modal-master-key').textContent = newMasterKey;
                document.getElementById('security-modal').classList.remove('hidden');
            }
        } else {
            statusEl.textContent = "保存失败。";
            statusEl.className = "mt-2 text-sm text-red-600";
        }
    } catch (e) {
        statusEl.textContent = "错误：" + e.message;
        statusEl.className = "mt-2 text-sm text-red-600";
    }
}

async function postJson(url, payload) {
    const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
    });
    if (res.ok) {
        const data = await res.json();
        return { ok: true, data };
    }
    const text = await res.text();
    return { ok: false, error: text || `HTTP ${res.status}` };
}

async function handleToolEncrypt(e) {
    e.preventDefault();
    const statusEl = document.getElementById('encrypt-status');
    setStatus(statusEl, '正在执行…', null);

    const inBase = document.getElementById('encrypt-in-base').value;
    const inRel = document.getElementById('encrypt-in-rel').value;
    const outBase = document.getElementById('encrypt-out-base').value;
    const outSub = document.getElementById('encrypt-out-subdir').value;

    const input = joinPath(inBase, inRel);
    const outputDir = joinPath(outBase, outSub);

    if (!input || !outputDir) {
        setStatus(statusEl, '请输入输入路径并选择输出目录。', false);
        return;
    }

    const resp = await postJson('/api/v1/tools/encrypt', { input, output_dir: outputDir });
    if (!resp.ok) {
        setStatus(statusEl, `执行失败：${resp.error}`, false);
        return;
    }
    setStatus(statusEl, `执行成功：已加密到 ${resp.data.details && resp.data.details.output_dir ? resp.data.details.output_dir : outputDir}`, true);
}

async function handleToolExport(e) {
    e.preventDefault();
    const statusEl = document.getElementById('export-status');
    setStatus(statusEl, '正在执行…', null);

    const outBase = document.getElementById('export-out-base').value;
    const outSub = document.getElementById('export-out-subdir').value;
    const shareKey = document.getElementById('export-share-key').value;

    const outputDir = joinPath(outBase, outSub);
    if (!outputDir) {
        setStatus(statusEl, '请选择导出目录。', false);
        return;
    }

    const resp = await postJson('/api/v1/tools/export', { output_dir: outputDir, share_key: shareKey });
    if (!resp.ok) {
        setStatus(statusEl, `执行失败：${resp.error}`, false);
        return;
    }
    const tarPath = resp.data.details && resp.data.details.tar_path ? resp.data.details.tar_path : '';
    const usedKey = resp.data.details && resp.data.details.share_key ? resp.data.details.share_key : '';
    const msg = tarPath ? `执行成功：${tarPath}${usedKey ? `（分享密钥：${usedKey}）` : ''}` : '执行成功';
    setStatus(statusEl, msg, true);
}

async function handleToolImport(e) {
    e.preventDefault();
    const statusEl = document.getElementById('import-status');

    const inBase = document.getElementById('import-in-base').value;
    const inRel = document.getElementById('import-in-rel').value;
    const shareKey = document.getElementById('import-share-key').value;
    const input = joinPath(inBase, inRel);

    if (!input || !shareKey) {
        setStatus(statusEl, '请选择分享包并输入分享密钥。', false);
        return;
    }

    setStatus(statusEl, '等待确认…', null);

    const msg = "您正在执行高风险操作：导入分享包会写入/覆盖本地元数据。<br>请确认您理解后果，并已备份元数据。";
    const hint = "<strong>建议：</strong><br>1. 在导入前先停止挂载与相关写入。<br>2. 如需回滚，请提前备份 metadata 目录。";

    openConfirmModal(
        "⚠️ 极度危险操作确认",
        msg,
        hint,
        async () => {
            setStatus(statusEl, '正在执行…', null);
            const resp = await postJson('/api/v1/tools/import', { input, share_key: shareKey });
            if (!resp.ok) {
                setStatus(statusEl, `执行失败：${resp.error}`, false);
                return;
            }
            setStatus(statusEl, '执行成功：已导入分享包。', true);
        }
    );
}
