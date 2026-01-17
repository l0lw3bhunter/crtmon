package main

func init() {
	dashboardHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>CRTMon - Admin Panel</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #1e3a8a 0%, #0f172a 100%);
            color: #e0e7ff;
            min-height: 100vh;
        }

        .login-container {
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
        }

        .login-box {
            background: rgba(30, 41, 59, 0.95);
            padding: 40px;
            border-radius: 10px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
            width: 100%;
            max-width: 400px;
            border: 1px solid rgba(148, 163, 184, 0.2);
        }

        .login-box h1 {
            margin-bottom: 30px;
            text-align: center;
            color: #60a5fa;
        }

        .form-group {
            margin-bottom: 20px;
        }

        .form-group label {
            display: block;
            margin-bottom: 8px;
            font-weight: 600;
        }

        .form-group input {
            width: 100%;
            padding: 12px;
            background: rgba(15, 23, 42, 0.8);
            border: 1px solid rgba(148, 163, 184, 0.3);
            border-radius: 6px;
            color: #e0e7ff;
            font-size: 14px;
        }

        .form-group input:focus {
            outline: none;
            border-color: #60a5fa;
            box-shadow: 0 0 0 3px rgba(96, 165, 250, 0.1);
        }

        .btn {
            width: 100%;
            padding: 12px;
            background: #3b82f6;
            border: none;
            border-radius: 6px;
            color: white;
            font-weight: 600;
            cursor: pointer;
            font-size: 14px;
            transition: all 0.3s ease;
        }

        .btn:hover {
            background: #2563eb;
            transform: translateY(-2px);
        }

        .btn:active {
            transform: translateY(0);
        }

        .error-message {
            background: rgba(239, 68, 68, 0.2);
            border: 1px solid rgba(239, 68, 68, 0.5);
            color: #fca5a5;
            padding: 12px;
            border-radius: 6px;
            margin-bottom: 20px;
            display: none;
        }

        .dashboard {
            display: none;
        }

        .header {
            background: rgba(15, 23, 42, 0.8);
            border-bottom: 1px solid rgba(148, 163, 184, 0.2);
            padding: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .header h1 {
            color: #60a5fa;
        }

        .logout-btn {
            padding: 8px 16px;
            background: #ef4444;
            border: none;
            border-radius: 6px;
            color: white;
            cursor: pointer;
            font-weight: 600;
            transition: all 0.3s ease;
        }

        .logout-btn:hover {
            background: #dc2626;
        }

        .sidebar {
            position: fixed;
            left: 0;
            top: 80px;
            width: 200px;
            background: rgba(15, 23, 42, 0.9);
            border-right: 1px solid rgba(148, 163, 184, 0.2);
            height: calc(100vh - 80px);
            padding: 20px;
            overflow-y: auto;
        }

        .sidebar a, .sidebar button {
            display: block;
            width: 100%;
            padding: 12px 16px;
            margin-bottom: 8px;
            background: rgba(30, 41, 59, 0.6);
            border: 1px solid rgba(148, 163, 184, 0.2);
            border-radius: 6px;
            color: #e0e7ff;
            text-decoration: none;
            cursor: pointer;
            text-align: left;
            font-size: 14px;
            transition: all 0.3s ease;
        }

        .sidebar a:hover, .sidebar button:hover {
            background: #3b82f6;
            border-color: #3b82f6;
            color: white;
        }

        .sidebar a.active {
            background: #3b82f6;
            border-color: #3b82f6;
            color: white;
        }

        .main-content {
            margin-left: 200px;
            margin-top: 80px;
            padding: 20px;
        }

        .content-section {
            display: none;
        }

        .content-section.active {
            display: block;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }

        .stat-card {
            background: rgba(30, 41, 59, 0.8);
            border: 1px solid rgba(148, 163, 184, 0.2);
            border-radius: 10px;
            padding: 20px;
            transition: all 0.3s ease;
        }

        .stat-card:hover {
            border-color: #60a5fa;
            box-shadow: 0 0 20px rgba(96, 165, 250, 0.1);
        }

        .stat-label {
            color: #94a3b8;
            font-size: 12px;
            text-transform: uppercase;
            letter-spacing: 1px;
            margin-bottom: 8px;
        }

        .stat-value {
            font-size: 28px;
            font-weight: 700;
            color: #60a5fa;
        }

        .stat-unit {
            color: #64748b;
            font-size: 12px;
            margin-top: 4px;
        }

        .chart-container {
            background: rgba(30, 41, 59, 0.8);
            border: 1px solid rgba(148, 163, 184, 0.2);
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 20px;
            position: relative;
            height: 300px;
        }

        .chart-title {
            margin-bottom: 15px;
            color: #e0e7ff;
            font-weight: 600;
        }

        .table-container {
            background: rgba(30, 41, 59, 0.8);
            border: 1px solid rgba(148, 163, 184, 0.2);
            border-radius: 10px;
            overflow: hidden;
        }

        table {
            width: 100%;
            border-collapse: collapse;
        }

        thead {
            background: rgba(15, 23, 42, 0.8);
        }

        th {
            padding: 15px;
            text-align: left;
            color: #60a5fa;
            font-weight: 600;
            border-bottom: 1px solid rgba(148, 163, 184, 0.2);
        }

        td {
            padding: 15px;
            border-bottom: 1px solid rgba(148, 163, 184, 0.1);
        }

        tr:hover {
            background: rgba(59, 130, 246, 0.05);
        }

        .badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
        }

        .badge-success {
            background: rgba(34, 197, 94, 0.2);
            color: #86efac;
        }

        .badge-danger {
            background: rgba(239, 68, 68, 0.2);
            color: #fca5a5;
        }

        .badge-warning {
            background: rgba(251, 146, 60, 0.2);
            color: #fdba74;
        }

        .action-buttons {
            display: flex;
            gap: 8px;
        }

        .action-btn {
            padding: 6px 12px;
            font-size: 12px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            transition: all 0.3s ease;
            font-weight: 600;
        }

        .action-btn-danger {
            background: rgba(239, 68, 68, 0.2);
            color: #fca5a5;
            border: 1px solid rgba(239, 68, 68, 0.3);
        }

        .action-btn-danger:hover {
            background: rgba(239, 68, 68, 0.3);
        }

        .action-btn-primary {
            background: rgba(59, 130, 246, 0.2);
            color: #93c5fd;
            border: 1px solid rgba(59, 130, 246, 0.3);
        }

        .action-btn-primary:hover {
            background: rgba(59, 130, 246, 0.3);
        }

        .form-wrapper {
            background: rgba(30, 41, 59, 0.8);
            border: 1px solid rgba(148, 163, 184, 0.2);
            border-radius: 10px;
            padding: 20px;
            margin-bottom: 20px;
            max-width: 500px;
        }

        .form-wrapper .form-group {
            margin-bottom: 15px;
        }

        .form-wrapper label {
            display: block;
            margin-bottom: 8px;
            font-weight: 600;
            color: #e0e7ff;
        }

        .form-wrapper input {
            width: 100%;
            padding: 10px;
            background: rgba(15, 23, 42, 0.8);
            border: 1px solid rgba(148, 163, 184, 0.3);
            border-radius: 6px;
            color: #e0e7ff;
            font-size: 14px;
        }

        .success-message {
            background: rgba(34, 197, 94, 0.2);
            border: 1px solid rgba(34, 197, 94, 0.5);
            color: #86efac;
            padding: 12px;
            border-radius: 6px;
            margin-bottom: 20px;
            display: none;
        }

        .loading {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid rgba(96, 165, 250, 0.3);
            border-radius: 50%;
            border-top-color: #60a5fa;
            animation: spin 1s linear infinite;
        }

        @keyframes spin {
            to { transform: rotate(360deg); }
        }

        @media (max-width: 768px) {
            .sidebar {
                width: 150px;
            }

            .main-content {
                margin-left: 150px;
            }

            .stats-grid {
                grid-template-columns: 1fr;
            }

            .header {
                flex-direction: column;
                gap: 10px;
            }
        }
    </style>
</head>
<body>
    <!-- Login Screen -->
    <div id="loginScreen" class="login-container">
        <div class="login-box">
            <h1>CRTMon Admin</h1>
            <div class="error-message" id="loginError"></div>
            <form id="loginForm">
                <div class="form-group">
                    <label for="password">Password</label>
                    <input type="password" id="password" placeholder="Enter admin password" required>
                </div>
                <button type="submit" class="btn">Login</button>
            </form>
        </div>
    </div>

    <!-- Dashboard -->
    <div id="dashboardScreen" class="dashboard">
        <div class="header">
            <h1>CRTMon Admin Panel</h1>
            <button class="logout-btn" onclick="logout()">Logout</button>
        </div>

        <div class="sidebar">
            <a href="#" onclick="switchTab('dashboard')" class="nav-link active" data-tab="dashboard">Dashboard</a>
            <a href="#" onclick="switchTab('domains')" class="nav-link" data-tab="domains">Domains</a>
            <a href="#" onclick="switchTab('targets')" class="nav-link" data-tab="targets">Targets</a>
            <a href="#" onclick="switchTab('blacklist')" class="nav-link" data-tab="blacklist">Blacklist</a>
            <a href="#" onclick="switchTab('config')" class="nav-link" data-tab="config">Configuration</a>
            <a href="#" onclick="switchTab('webhooks')" class="nav-link" data-tab="webhooks">Webhooks</a>
        </div>

        <div class="main-content">
            <!-- Dashboard Section -->
            <div id="dashboard" class="content-section active">
                <h2>System Status</h2>
                <div class="stats-grid">
                    <div class="stat-card">
                        <div class="stat-label">Uptime</div>
                        <div class="stat-value" id="uptimeValue">--:--:--</div>
                        <div class="stat-unit">hours:minutes:seconds</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Memory Usage</div>
                        <div class="stat-value" id="memoryValue">-- MB</div>
                        <div class="stat-unit">allocated memory</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Total Domains</div>
                        <div class="stat-value" id="domainsCountValue">--</div>
                        <div class="stat-unit">tracked domains</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Total Hits</div>
                        <div class="stat-value" id="totalHitsValue">--</div>
                        <div class="stat-unit">certificate transparency hits</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Avg Hits/Domain</div>
                        <div class="stat-value" id="avgHitsValue">--</div>
                        <div class="stat-unit">average per domain</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Discovered (24h)</div>
                        <div class="stat-value" id="discovered24hValue">--</div>
                        <div class="stat-unit">new in last 24 hours</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">High Risk</div>
                        <div class="stat-value" id="highRiskValue" style="color: #fca5a5;">--</div>
                        <div class="stat-unit">risk score ≥50</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Wildcards</div>
                        <div class="stat-value" id="wildcardsValue" style="color: #fdba74;">--</div>
                        <div class="stat-unit">wildcard certificates</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Blacklisted</div>
                        <div class="stat-value" id="blacklistedValue">--</div>
                        <div class="stat-unit">spam domains</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Duplicates</div>
                        <div class="stat-value" id="duplicatesValue">--</div>
                        <div class="stat-unit">marked as noise</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Active Targets</div>
                        <div class="stat-value" id="targetsValue">--</div>
                        <div class="stat-unit">being monitored</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">CT Logs Active</div>
                        <div class="stat-value" id="ctLogsValue" style="color: #86efac;">--</div>
                        <div class="stat-unit">connected / disconnected</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Scan Queue</div>
                        <div class="stat-value" id="scanQueueValue" style="color: #93c5fd;">--</div>
                        <div class="stat-unit">active enumeration jobs</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Resolution Rate</div>
                        <div class="stat-value" id="resolutionRateValue">--</div>
                        <div class="stat-unit">domains that resolve</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">Enum Success</div>
                        <div class="stat-value" id="enumSuccessValue">--</div>
                        <div class="stat-unit">scan completion rate</div>
                    </div>
                </div>

                <div class="chart-container">
                    <div class="chart-title">Top 10 Domains by Hit Count</div>
                    <canvas id="topDomainsChart"></canvas>
                </div>

                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin-bottom: 20px;">
                    <div class="chart-container">
                        <div class="chart-title">Status Code Distribution</div>
                        <canvas id="statusCodeChart"></canvas>
                    </div>
                    <div class="chart-container">
                        <div class="chart-title">Discovery Rate (Domains/Hour)</div>
                        <canvas id="discoveryRateChart"></canvas>
                    </div>
                </div>

                <div class="chart-container">
                    <div class="chart-title">Top Targets by Subdomain Count</div>
                    <canvas id="topTargetsChart"></canvas>
                </div>
            </div>

            <!-- Domains Section -->
            <div id="domains" class="content-section">
                <h2>Tracked Domains</h2>
                <div class="table-container">
                    <table>
                        <thead>
                            <tr>
                                <th>Domain</th>
                                <th>Hits</th>
                                <th>Risk</th>
                                <th>Labels</th>
                                <th>First Seen</th>
                                <th>Last Seen</th>
                                <th>Status</th>
                            </tr>
                        </thead>
                        <tbody id="domainsTable">
                            <tr><td colspan="7" style="text-align: center; padding: 20px;">Loading...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <!-- Targets Section -->
            <div id="targets" class="content-section">
                <h2>Manage Targets</h2>
                <div class="form-wrapper">
                    <h3 style="margin-bottom: 15px; color: #60a5fa;">Add New Target</h3>
                    <div class="success-message" id="targetSuccess"></div>
                    <form id="addTargetForm" style="display: flex; gap: 10px;">
                        <input type="text" id="newTarget" placeholder="example.com" style="flex: 1;" required>
                        <button type="submit" class="btn" style="width: auto;">Add</button>
                    </form>
                </div>

                <h3 style="margin-bottom: 15px; color: #60a5fa; margin-top: 30px;">Current Targets</h3>
                <div class="table-container">
                    <table>
                        <thead>
                            <tr>
                                <th>Target</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody id="targetsTable">
                            <tr><td colspan="2" style="text-align: center; padding: 20px;">Loading...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <!-- Blacklist Section -->
            <div id="blacklist" class="content-section">
                <h2>Blacklisted Domains</h2>
                <div class="table-container">
                    <table>
                        <thead>
                            <tr>
                                <th>Domain</th>
                                <th>Hits</th>
                                <th>Blacklisted Date</th>
                                <th>Consecutive Days >10 hits</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody id="blacklistTable">
                            <tr><td colspan="5" style="text-align: center; padding: 20px;">Loading...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <!-- Config Section -->
            <div id="config" class="content-section">
                <h2>Configuration</h2>
                <div class="table-container">
                    <table>
                        <thead>
                            <tr>
                                <th>Setting</th>
                                <th>Value</th>
                            </tr>
                        </thead>
                        <tbody id="configTable">
                            <tr><td colspan="2" style="text-align: center; padding: 20px;">Loading...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <!-- Webhooks Section -->
            <div id="webhooks" class="content-section">
                <h2>Webhooks</h2>
                <div class="form-wrapper" style="margin-bottom: 20px;">
                    <h3 style="margin-bottom: 15px; color: #60a5fa;">Notification Endpoints</h3>
                    <form id="webhookForm" style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px;">
                        <div class="form-group" style="grid-column: span 2;">
                            <label for="mainWebhook">Discord Webhook (primary)</label>
                            <input type="text" id="mainWebhook" placeholder="https://discord.com/api/webhooks/...">
                            <div class="action-buttons" style="justify-content: flex-end; margin-top: 8px;">
                                <button type="button" class="action-btn action-btn-primary" onclick="testWebhook('main')">Test</button>
                            </div>
                        </div>

                        <div class="form-group">
                            <label for="telegramBot">Telegram Bot Token</label>
                            <input type="text" id="telegramBot" placeholder="1234567890:ABCDEF...">
                            <div class="action-buttons" style="justify-content: flex-end; margin-top: 8px;">
                                <button type="button" class="action-btn action-btn-primary" onclick="testWebhook('telegram')">Test</button>
                            </div>
                        </div>
                        <div class="form-group">
                            <label for="telegramChat">Telegram Chat ID</label>
                            <input type="text" id="telegramChat" placeholder="-1001234567890">
                        </div>

                        <div class="form-group" style="grid-column: span 2; margin-top: 10px;">
                            <h3 style="margin-bottom: 10px; color: #60a5fa;">Custom Webhooks</h3>
                        </div>

                        <div class="form-group">
                            <label for="newDomainsWebhook">New Domains Webhook</label>
                            <input type="text" id="newDomainsWebhook" placeholder="https://...">
                            <div class="action-buttons" style="justify-content: flex-end; margin-top: 8px;">
                                <button type="button" class="action-btn action-btn-primary" onclick="testWebhook('new_domains')">Test</button>
                            </div>
                        </div>
                        <div class="form-group">
                            <label for="subdomainScansWebhook">Subdomain Scans Webhook</label>
                            <input type="text" id="subdomainScansWebhook" placeholder="https://...">
                            <div class="action-buttons" style="justify-content: flex-end; margin-top: 8px;">
                                <button type="button" class="action-btn action-btn-primary" onclick="testWebhook('subdomain_scans')">Test</button>
                            </div>
                        </div>
                        <div class="form-group">
                            <label for="directoryScansWebhook">Directory Scans Webhook</label>
                            <input type="text" id="directoryScansWebhook" placeholder="https://...">
                            <div class="action-buttons" style="justify-content: flex-end; margin-top: 8px;">
                                <button type="button" class="action-btn action-btn-primary" onclick="testWebhook('directory_scans')">Test</button>
                            </div>
                        </div>
                        <div class="form-group">
                            <label for="dailySummaryWebhook">Daily Summary Webhook</label>
                            <input type="text" id="dailySummaryWebhook" placeholder="https://...">
                            <div class="action-buttons" style="justify-content: flex-end; margin-top: 8px;">
                                <button type="button" class="action-btn action-btn-primary" onclick="testWebhook('daily_summary')">Test</button>
                            </div>
                        </div>

                        <div style="grid-column: span 2; display: flex; justify-content: flex-end; margin-top: 10px;">
                            <button type="submit" class="btn" style="width: auto;">Save Changes</button>
                        </div>
                    </form>
                </div>
            </div>
        </div>
    </div>

    <script src="/assets/app.js"></script>
</body>
</html>
`
}
