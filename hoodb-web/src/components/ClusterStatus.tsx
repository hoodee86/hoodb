import React, { useState, useEffect } from 'react';
import {
  Card,
  CardContent,
  Typography,
  Box,
  Alert,
  Chip,
  Grid,
  Paper,
  Button,
  CircularProgress,
  LinearProgress,
} from '@mui/material';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ErrorIcon from '@mui/icons-material/Error';
import RefreshIcon from '@mui/icons-material/Refresh';
import LeaderboardIcon from '@mui/icons-material/Leaderboard';
import { useTranslation } from 'react-i18next';
import { getClusterStats, healthCheck } from '../services/api';

const ClusterStatus: React.FC = () => {
  const { t } = useTranslation();
  const [stats, setStats] = useState<any>(null);
  const [health, setHealth] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [autoRefresh, setAutoRefresh] = useState(false);

  const fetchStatus = async () => {
    setLoading(true);
    setError('');
    try {
      const [statsData, healthData] = await Promise.all([
        getClusterStats(),
        healthCheck(),
      ]);
      setStats(statsData);
      setHealth(healthData);
    } catch (err: any) {
      setError(err.response?.data?.error || err.message || t('clusterFetchFailed'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStatus();
  }, []);

  useEffect(() => {
    if (autoRefresh) {
      const interval = setInterval(fetchStatus, 3000);
      return () => clearInterval(interval);
    }
  }, [autoRefresh]);

  const getStateColor = (state: string) => {
    switch (state) {
      case 'Leader':
        return 'success';
      case 'Follower':
        return 'primary';
      case 'Candidate':
        return 'warning';
      default:
        return 'default';
    }
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h5">
          {t('clusterTitle')}
        </Typography>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Button
            variant={autoRefresh ? 'contained' : 'outlined'}
            size="small"
            onClick={() => setAutoRefresh(!autoRefresh)}
          >
            {autoRefresh ? t('clusterAutoRefreshOn') : t('clusterAutoRefreshOff')}
          </Button>
          <Button
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={fetchStatus}
            disabled={loading}
            size="small"
          >
            {t('clusterRefresh')}
          </Button>
        </Box>
      </Box>

      {loading && !stats && (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
          <CircularProgress />
        </Box>
      )}

      {error && (
        <Alert severity="error" icon={<ErrorIcon />}>
          {error}
        </Alert>
      )}

      {stats && (
        <Grid container spacing={3}>
          <Grid size={{ xs: 12, md: 6 }}>
            <Card elevation={2}>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  {t('clusterNodeStatus')}
                </Typography>
                <Box sx={{ py: 2 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
                    <Typography color="text.secondary">{t('clusterState')}</Typography>
                    <Chip
                      label={stats.state}
                      color={getStateColor(stats.state)}
                      icon={stats.isLeader ? <LeaderboardIcon /> : undefined}
                    />
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
                    <Typography color="text.secondary">{t('clusterIsLeader')}</Typography>
                    <Chip
                      label={stats.isLeader ? t('clusterYes') : t('clusterNo')}
                      color={stats.isLeader ? 'success' : 'default'}
                      size="small"
                    />
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
                    <Typography color="text.secondary">{t('clusterLeaderAddress')}</Typography>
                    <Typography fontFamily="monospace">{stats.leader || t('clusterUnknown')}</Typography>
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
                    <Typography color="text.secondary">{t('clusterHealth')}</Typography>
                    <Chip
                      label={health?.status || t('clusterUnknown')}
                      color={health?.status === 'healthy' ? 'success' : 'error'}
                      icon={health?.status === 'healthy' ? <CheckCircleIcon /> : <ErrorIcon />}
                      size="small"
                    />
                  </Box>
                </Box>
              </CardContent>
            </Card>
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <Card elevation={2}>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  {t('clusterRaftMetrics')}
                </Typography>
                <Box sx={{ py: 2 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
                    <Typography color="text.secondary">{t('clusterLastIndex')}</Typography>
                    <Typography fontFamily="monospace">{stats.lastIndex?.toLocaleString()}</Typography>
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 2 }}>
                    <Typography color="text.secondary">{t('clusterAppliedIndex')}</Typography>
                    <Typography fontFamily="monospace">{stats.appliedIndex?.toLocaleString()}</Typography>
                  </Box>
                  <Box sx={{ mb: 2 }}>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                      <Typography color="text.secondary">{t('clusterApplyProgress')}</Typography>
                      <Typography variant="body2">
                        {stats.lastIndex > 0
                          ? ((stats.appliedIndex / stats.lastIndex) * 100).toFixed(1)
                          : 0}
                        %
                      </Typography>
                    </Box>
                    <LinearProgress
                      variant="determinate"
                      value={
                        stats.lastIndex > 0
                          ? (stats.appliedIndex / stats.lastIndex) * 100
                          : 0
                      }
                    />
                  </Box>
                </Box>
              </CardContent>
            </Card>
          </Grid>

          <Grid size={{ xs: 12 }}>
            <Card elevation={2}>
              <CardContent>
                <Typography variant="h6" gutterBottom>
                  {t('clusterDetailedStats')}
                </Typography>
                <Paper variant="outlined" sx={{ p: 2, bgcolor: 'grey.50' }}>
                  <Grid container spacing={2}>
                    {stats.raftStats &&
                      Object.entries(stats.raftStats).map(([key, value]) => (
                        <Grid size={{ xs: 12, sm: 6, md: 4 }} key={key}>
                          <Box>
                            <Typography variant="caption" color="text.secondary">
                              {key}
                            </Typography>
                            <Typography variant="body2" fontFamily="monospace">
                              {String(value)}
                            </Typography>
                          </Box>
                        </Grid>
                      ))}
                  </Grid>
                </Paper>
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      )}

      <Card elevation={2} sx={{ mt: 3 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            {t('clusterAbout')}
          </Typography>
          <Typography variant="body2" color="text.secondary" paragraph>
            {t('clusterAbout1')}
          </Typography>
          <Typography variant="body2" color="text.secondary" paragraph>
            {t('clusterAbout2')}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {t('clusterAbout3')}
          </Typography>
        </CardContent>
      </Card>
    </Box>
  );
};

export default ClusterStatus;
