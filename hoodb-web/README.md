# HooDB Web Interface

React-based web UI for testing and managing HooDB distributed database.

## Features

- **KV Operations**: Create, Read, Update, Delete key-value pairs
- **Batch Operations**: High-performance batch writes (~5,000 ops/sec)
- **Cluster Status**: Real-time cluster health and Raft metrics
- **Performance Test**: Benchmark sequential and batch write performance

## Prerequisites

- Node.js 14+ and npm
- HooDB cluster running (default: http://localhost:8001)

## Quick Start

```bash
# Install dependencies
npm install

# Start development server
npm start
```

The app will open at http://localhost:3000

## Configuration

Edit `.env` to change the API endpoint:

```
REACT_APP_API_URL=http://localhost:8001
```

## Build for Production

```bash
npm run build
```

Output will be in the `build/` directory.

## Technology Stack

- React 18 with TypeScript
- Material-UI (Google Material Design)
- Axios for API calls
- Create React App

## Project Structure

```
src/
├── components/         # React components
│   ├── KVOperations.tsx
│   ├── BatchOperations.tsx
│   ├── ClusterStatus.tsx
│   └── PerformanceTest.tsx
├── services/          # API service layer
│   └── api.ts
└── App.tsx           # Main app component
```

## Usage

### KV Operations
- Enter a key and value
- Click "Set" to store, "Get" to retrieve, or "Delete" to remove
- Use the example buttons for quick testing

### Batch Operations
- Add multiple key-value pairs
- Click "Batch Set" to write all pairs in a single Raft commit
- Use "Load Example" or "Generate 50 pairs" for testing

### Cluster Status
- View current node state (Leader/Follower)
- Monitor Raft metrics (lastIndex, appliedIndex)
- Enable auto-refresh for real-time monitoring

### Performance Test
- Choose number of operations
- Run "Sequential Test" for single-write performance (~50 ops/sec)
- Run "Batch Test" for optimized performance (~5,000 ops/sec)

## API Endpoints Used

- `GET /kv/:key` - Get value
- `PUT /kv/:key` - Set value
- `DELETE /kv/:key` - Delete key
- `POST /kv/batch` - Batch set
- `GET /cluster/stats` - Cluster statistics
- `GET /health` - Health check

## License

Same as HooDB project.
