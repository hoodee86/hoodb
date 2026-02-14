import React, { useState, useRef } from 'react';
import {
  Card,
  CardContent,
  Typography,
  Box,
  Button,
  TextField,
  Alert,
  Grid,
  Paper,
  LinearProgress,
  Chip,
} from '@mui/material';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import StopIcon from '@mui/icons-material/Stop';
import SpeedIcon from '@mui/icons-material/Speed';
import { useTranslation } from 'react-i18next';
import { batchSet, runBenchmark } from '../services/api';

interface TestResult {
  type: string;
  count: number;
  duration: number;
  opsPerSec: number;
  success: number;
  failed: number;
}

const PerformanceTest: React.FC = () => {
  const { t } = useTranslation();
  const [numOps, setNumOps] = useState('1000');
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState(0);
  const [result, setResult] = useState<TestResult | null>(null);
  const [error, setError] = useState('');
  const [aborted, setAborted] = useState(false);
  const abortedRef = useRef(false);

  const runSequentialTest = async () => {
    const count = parseInt(numOps);
    if (isNaN(count) || count <= 0) {
      setError(t('perfInvalidNumber'));
      return;
    }

    setRunning(true);
    setAborted(false);
    abortedRef.current = false;
    setError('');
    setResult(null);
    setProgress(0);

    try {
      // 使用服务端基准测试接口, 绕过浏览器 6 连接限制
      setProgress(50); // 服务端执行中
      const res = await runBenchmark(count, 50);

      setResult({
        type: t('perfSequentialWrite'),
        count: res.count,
        duration: res.duration_ms / 1000,
        opsPerSec: res.ops_per_sec,
        success: res.success,
        failed: res.failed,
      });
    } catch (err: any) {
      setError(err.message || t('perfTestFailed'));
    } finally {
      setRunning(false);
      setProgress(0);
    }
  };

  const runBatchTest = async () => {
    const count = parseInt(numOps);
    if (isNaN(count) || count <= 0) {
      setError(t('perfInvalidNumber'));
      return;
    }

    setRunning(true);
    setAborted(false);
    abortedRef.current = false;
    setError('');
    setResult(null);
    setProgress(0);

    const batchSize = 50;
    const batches = Math.ceil(count / batchSize);
    let success = 0;
    let failed = 0;
    const startTime = Date.now();

    try {
      for (let b = 0; b < batches; b++) {
        if (abortedRef.current) break;

        const items: { [key: string]: string } = {};
        const currentBatchSize = Math.min(batchSize, count - b * batchSize);

        for (let i = 0; i < currentBatchSize; i++) {
          const idx = b * batchSize + i;
          items[`batch_test_${Date.now()}_${idx}`] = `value_${idx}`;
        }

        try {
          await batchSet(items);
          success += currentBatchSize;
        } catch {
          failed += currentBatchSize;
        }

        setProgress(((b + 1) / batches) * 100);
      }

      const duration = (Date.now() - startTime) / 1000;
      setResult({
        type: t('perfBatchWrite'),
        count,
        duration,
        opsPerSec: success / duration,
        success,
        failed,
      });
    } catch (err: any) {
      setError(err.message || t('perfTestFailed'));
    } finally {
      setRunning(false);
      setProgress(0);
    }
  };

  const handleAbort = () => {
    abortedRef.current = true;
    setAborted(true);
  };

  return (
    <Box>
      <Typography variant="h5" gutterBottom>
        {t('perfTitle')}
      </Typography>
      <Typography variant="body2" color="text.secondary" paragraph>
        {t('perfDescription')}
      </Typography>

      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 6 }}>
          <Card elevation={2}>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                {t('perfTestConfig')}
              </Typography>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <TextField
                  label={t('perfNumOps')}
                  type="number"
                  value={numOps}
                  onChange={(e) => setNumOps(e.target.value)}
                  disabled={running}
                  fullWidth
                />

                <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                  <Button
                    variant="contained"
                    startIcon={<PlayArrowIcon />}
                    onClick={runSequentialTest}
                    disabled={running}
                  >
                    {t('perfSequentialTest')}
                  </Button>
                  <Button
                    variant="contained"
                    color="secondary"
                    startIcon={<SpeedIcon />}
                    onClick={runBatchTest}
                    disabled={running}
                  >
                    {t('perfBatchTest')}
                  </Button>
                  {running && (
                    <Button
                      variant="outlined"
                      color="error"
                      startIcon={<StopIcon />}
                      onClick={handleAbort}
                    >
                      {t('perfAbort')}
                    </Button>
                  )}
                </Box>

                {running && (
                  <Box>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                      <Typography variant="body2">{t('perfProgress')}</Typography>
                      <Typography variant="body2">{progress.toFixed(1)}%</Typography>
                    </Box>
                    <LinearProgress variant="determinate" value={progress} />
                  </Box>
                )}
              </Box>
            </CardContent>
          </Card>

          <Card elevation={2} sx={{ mt: 2 }}>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                {t('perfQuickPresets')}
              </Typography>
              <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                {[
                  { label: '100 ops', value: '100' },
                  { label: '500 ops', value: '500' },
                  { label: '1,000 ops', value: '1000' },
                  { label: '5,000 ops', value: '5000' },
                  { label: '10,000 ops', value: '10000' },
                ].map((preset) => (
                  <Chip
                    key={preset.value}
                    label={preset.label}
                    onClick={() => setNumOps(preset.value)}
                    disabled={running}
                    color={numOps === preset.value ? 'primary' : 'default'}
                    variant={numOps === preset.value ? 'filled' : 'outlined'}
                  />
                ))}
              </Box>
            </CardContent>
          </Card>
        </Grid>

        <Grid size={{ xs: 12, md: 6 }}>
          <Card elevation={2}>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                {t('perfTestResults')}
              </Typography>

              {error && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  {error}
                </Alert>
              )}

              {result && (
                <Paper variant="outlined" sx={{ p: 2 }}>
                  <Box sx={{ mb: 2 }}>
                    <Chip label={result.type} color="primary" sx={{ mb: 1 }} />
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                    <Typography color="text.secondary">{t('perfTotalOps')}</Typography>
                    <Typography fontWeight="bold">{result.count.toLocaleString()}</Typography>
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                    <Typography color="text.secondary">{t('perfDuration')}</Typography>
                    <Typography fontWeight="bold">{result.duration.toFixed(2)}s</Typography>
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                    <Typography color="text.secondary">{t('perfThroughput')}</Typography>
                    <Typography fontWeight="bold" color="primary.main">
                      {result.opsPerSec.toFixed(2)} ops/sec
                    </Typography>
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                    <Typography color="text.secondary">{t('perfSuccess')}</Typography>
                    <Typography color="success.main">{result.success.toLocaleString()}</Typography>
                  </Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                    <Typography color="text.secondary">{t('perfFailed')}</Typography>
                    <Typography color="error.main">{result.failed.toLocaleString()}</Typography>
                  </Box>
                </Paper>
              )}

              {!result && !error && (
                <Paper
                  variant="outlined"
                  sx={{
                    p: 3,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    minHeight: 200,
                    bgcolor: 'action.hover',
                  }}
                >
                  <Typography color="text.secondary">
                    {t('perfRunTest')}
                  </Typography>
                </Paper>
              )}
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      <Card elevation={2} sx={{ mt: 3 }}>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            {t('perfExpected')}
          </Typography>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: 6 }}>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography variant="subtitle2" gutterBottom>
                  {t('perfSequentialWrite')}
                </Typography>
                <Typography variant="h4" color="primary">
                  ~1,000+
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {t('perfOpsSecRaft')}
                </Typography>
              </Paper>
            </Grid>
            <Grid size={{ xs: 12, sm: 6 }}>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Typography variant="subtitle2" gutterBottom>
                  {t('perfBatchWrite')}
                </Typography>
                <Typography variant="h4" color="secondary">
                  ~10,000+
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {t('perfOpsSecBatch')}
                </Typography>
              </Paper>
            </Grid>
          </Grid>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            {t('perfTip')}
          </Typography>
        </CardContent>
      </Card>
    </Box>
  );
};

export default PerformanceTest;
