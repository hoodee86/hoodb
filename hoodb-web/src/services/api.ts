import axios from 'axios';

const API_BASE_URL = process.env.REACT_APP_API_URL || 'http://localhost:8001/api/v1';

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
});

export interface KVPair {
  key: string;
  value: string;
}

export interface BatchItems {
  items: { [key: string]: string };
}

export interface ClusterStats {
  isLeader: boolean;
  leader: string;
  state: string;
  lastIndex: number;
  appliedIndex: number;
  raftStats: { [key: string]: string };
}

// KV Operations
export const getKey = async (key: string): Promise<string> => {
  const response = await api.get(`/kv/${key}`);
  return response.data.value;
};

export const setKey = async (key: string, value: string): Promise<void> => {
  await api.put(`/kv/${key}`, { value });
};

export const deleteKey = async (key: string): Promise<void> => {
  await api.delete(`/kv/${key}`);
};

// Batch Operations
export const batchSet = async (items: { [key: string]: string }): Promise<void> => {
  await api.post('/kv/batch', { items });
};

// Cluster Operations
export const getClusterStats = async (): Promise<ClusterStats> => {
  const response = await api.get('/cluster/stats');
  return response.data;
};

export const healthCheck = async (): Promise<{ status: string }> => {
  // /health 是根级别端点，不在 /api/v1 下
  const baseHost = (process.env.REACT_APP_API_URL || 'http://localhost:8001').replace(/\/api\/v1\/?$/, '');
  const response = await axios.get(`${baseHost}/health`);
  return response.data;
};

// Benchmark
export interface BenchmarkResult {
  count: number;
  success: number;
  failed: number;
  duration_ms: number;
  ops_per_sec: number;
}

export const runBenchmark = async (count: number, concurrency: number): Promise<BenchmarkResult> => {
  const response = await api.post('/benchmark', { count, concurrency }, { timeout: 120000 });
  return response.data;
};

export default api;
