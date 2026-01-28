/**
 * Centralized CSS selectors for OuroborosDB dashboard E2E tests.
 * These selectors match the structure in pkg/dashboard/static/index.html.
 */

export const selectors = {
  // Header & Connection
  connectionStatus: '#connection-status',
  connectionConnected: '#connection-status.connected',
  connectionDisconnected: '#connection-status.disconnected',

  // Tab Navigation
  tabs: {
    overview: '.tab[data-panel="overview"]',
    nodes: '.tab[data-panel="nodes"]',
    logs: '.tab[data-panel="logs"]',
    data: '.tab[data-panel="data"]',
    distribution: '.tab[data-panel="distribution"]',
    upload: '.tab[data-panel="upload"]',
  },

  // Panels
  panels: {
    overview: '#overview',
    nodes: '#nodes',
    logs: '#logs',
    data: '#data',
    distribution: '#distribution',
    upload: '#upload',
  },

  // Overview Panel
  overview: {
    nodeGrid: '#overview-nodes',
    nodeCard: '#overview-nodes .node-card',
    logsContainer: '#overview-logs',
  },

  // Nodes Panel
  nodes: {
    table: '#nodes-table',
    tableRows: '#nodes-table tr',
    subscribeButton: '#nodes-table button:has-text("Subscribe")',
  },

  // Logs Panel
  logs: {
    stream: '#log-stream',
    logEntry: '#log-stream .log-entry',
    filterNode: '#log-filter-node',
    filterLevel: '#log-filter-level',
    clearButton: 'button:has-text("Clear")',
    subscribeNodes: '#subscribe-nodes',
    subscribeCheckbox: '#subscribe-nodes input[type="checkbox"]',
  },

  // Data Browser Panel (Vertices)
  data: {
    vertexSearch: '#vertex-search',
    searchButton: 'button:has-text("Search")',
    verticesTable: '#vertices-table',
    verticesTableRows: '#vertices-table tr',
    pagination: '#vertices-pagination',
  },

  // Distribution Panel
  distribution: {
    overview: '#distribution-overview',
    table: '#distribution-table',
    // Counter cards
    totalNodesCard: '.node-card:has-text("Total Nodes")',
    totalVerticesCard: '.node-card:has-text("Total Vertices")',
    totalBlocksCard: '.node-card:has-text("Total Blocks")',
    totalSlicesCard: '.node-card:has-text("Total Slices")',
    counterValue: 'p', // The <p> inside each card contains the number
  },

  // Upload Panel
  upload: {
    zone: '#upload-zone',
    fileInput: '#file-input',
    flowCard: '#upload-flow-card',
    flow: '#upload-flow',
    flowStage: '#upload-flow .flow-stage',
    flowStageComplete: '#upload-flow .flow-stage.complete',
    flowStageInProgress: '#upload-flow .flow-stage.in-progress',
    flowStagePending: '#upload-flow .flow-stage.pending',
    hash: '#upload-flow .hash',
  },

  // Common
  card: '.card',
  hash: '.hash',
  button: {
    primary: '.btn-primary',
    secondary: '.btn-secondary',
    success: '.btn-success',
  },
} as const;

/**
 * Wait times for various dashboard operations (in milliseconds).
 */
export const timeouts = {
  // WebSocket connection establishment
  websocketConnect: 10000,

  // Cluster formation and node discovery
  clusterFormation: 15000,

  // Upload flow completion (all 5 stages)
  uploadComplete: 20000,

  // Log entry appearance after subscription
  logAppear: 5000,

  // API response
  apiResponse: 5000,

  // Small delay between operations
  operationDelay: 100,
} as const;

/**
 * Expected number of upload flow stages.
 */
export const UPLOAD_STAGES_COUNT = 5;

/**
 * Minimum expected nodes in cluster for E2E tests.
 */
export const MIN_CLUSTER_NODES = 3;
