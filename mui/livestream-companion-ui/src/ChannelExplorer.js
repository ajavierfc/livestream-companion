import React, { useCallback, useContext, useEffect, useRef, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Card,
  CardActions,
  CardContent,
  CardMedia,
  Chip,
  CircularProgress,
  Container,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  Grid,
  IconButton,
  InputAdornment,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import ClearIcon from '@mui/icons-material/Clear';
import LiveTvIcon from '@mui/icons-material/LiveTv';
import SearchIcon from '@mui/icons-material/Search';
import axios from './axiosSetup';
import mpegts from 'mpegts.js';
import VLCIcon from './VLCIcon';
import { SnackbarContext } from './SnackbarContext';
import { copyToClipboard } from './App';
import { createVLCIntentURL } from './streamUrls';

const PAGE_SIZE = 60;
const QUICK_SEARCHES_KEY = 'channel-explorer-quick-searches';
const MAX_QUICK_SEARCHES = 8;
const prefixColorByValue = new Map();
const qualityColors = {
  SD: '#616161',
  HD: '#1565c0',
  FHD: '#6a1b9a',
  UHD: '#00838f',
  '4K': '#2e7d32',
  '8K': '#ad1457',
  RAW: '#ef6c00',
  HEVC: '#5d4037',
};

const getChannelPresentation = (channel) => {
  let name = channel.name || '';
  let prefix = '';
  const prefixMatch = name.match(/^([A-Z0-9+]{1,12}):\s*/i);
  if (prefixMatch) {
    prefix = prefixMatch[1];
    name = name.slice(prefixMatch[0].length);
  }

  name = name.replace(/\s+[⁰¹²³⁴⁵⁶⁷⁸⁹]+\s*ᶠᵖˢ$/u, '');
  const qualityMatch = name.match(/\s+(SD|HD|FHD|UHD|4K|8K|RAW|HEVC|ᴿᴬᵂ|ᴴᴰ|ᵁᴴᴰ|ʰᵉᵛᶜ)$/iu);
  const quality = qualityMatch
    ? ({
      'ᴿᴬᵂ': 'RAW',
      'ᴴᴰ': 'HD',
      'ᵁᴴᴰ': 'UHD',
      'ʰᵉᵛᶜ': 'HEVC',
    }[qualityMatch[1]] || qualityMatch[1].toUpperCase())
    : '';
  if (qualityMatch) {
    name = name.slice(0, -qualityMatch[0].length);
  }

  return { name: name.trim() || channel.name || 'Unnamed channel', prefix, quality };
};

const getPrefixColor = value => {
  if (!prefixColorByValue.has(value)) {
    const hue = (prefixColorByValue.size * 137.508) % 360;
    prefixColorByValue.set(value, `hsl(${hue} 68% 38%)`);
  }
  return prefixColorByValue.get(value);
};

const readQuickSearches = () => {
  try {
    const saved = JSON.parse(localStorage.getItem(QUICK_SEARCHES_KEY) || '[]');
    return Array.isArray(saved)
      ? saved.filter(item => item && typeof item.term === 'string' && Number.isFinite(item.uses))
      : [];
  } catch (error) {
    console.warn('Unable to read saved channel searches', error);
    return [];
  }
};

const ChannelArtwork = ({ channel, prefix, quality }) => {
  const [imageFailed, setImageFailed] = useState(false);
  const image = channel.stream_icon || channel.StreamIcon;

  return (
    <Box sx={{ height: 120, position: 'relative', display: 'grid', placeItems: 'center', bgcolor: 'action.hover', overflow: 'hidden' }}>
      {image && !imageFailed ? (
        <CardMedia
          component="img"
          height="120"
          image={image}
          alt=""
          sx={{ objectFit: 'contain', p: 1, bgcolor: 'background.default' }}
          onError={() => setImageFailed(true)}
        />
      ) : (
        <LiveTvIcon color="disabled" sx={{ fontSize: 52 }} />
      )}
      {prefix && (
        <Chip
          size="small"
          label={prefix}
          sx={{
            position: 'absolute',
            top: 8,
            left: 8,
            color: 'common.white',
            bgcolor: getPrefixColor(prefix.toUpperCase()),
            fontWeight: 'bold',
          }}
        />
      )}
      {quality && (
        <Chip
          size="small"
          label={quality}
          sx={{
            position: 'absolute',
            top: 8,
            right: 8,
            color: 'common.white',
            bgcolor: qualityColors[quality] || '#455a64',
            fontWeight: 'bold',
          }}
        />
      )}
    </Box>
  );
};

const ChannelExplorer = () => {
  const [playlists, setPlaylists] = useState([]);
  const [categories, setCategories] = useState([]);
  const [playlistId, setPlaylistId] = useState('');
  const [categoryId, setCategoryId] = useState('');
  const [query, setQuery] = useState('');
  const [appliedSearch, setAppliedSearch] = useState(null);
  const [channels, setChannels] = useState([]);
  const [total, setTotal] = useState(0);
  const [prefixes, setPrefixes] = useState([]);
  const [qualities, setQualities] = useState([]);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [quickSearches, setQuickSearches] = useState(readQuickSearches);
  const [currentChannel, setCurrentChannel] = useState(null);
  const [openPlayer, setOpenPlayer] = useState(false);
  const videoRef = useRef(null);
  const requestIdRef = useRef(0);
  const playlistChangeRequestRef = useRef(0);
  const skipCategoryEffectRef = useRef('');
  const { openSnackbar } = useContext(SnackbarContext);

  useEffect(() => {
    let cancelled = false;
    axios.get('/api/playlists')
      .then(({ data }) => {
        if (!cancelled) {
          setPlaylists(data || []);
          const defaultPlaylist = data?.find(item => item.ImportStatus === 2) || data?.[0];
          setPlaylistId(defaultPlaylist?.ID || '');
        }
      })
      .catch(() => {
        if (!cancelled) setError('Unable to load playlists.');
      });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    if (!playlistId) {
      setCategories([]);
      setCategoryId('');
      setAppliedSearch(null);
      setChannels([]);
      setTotal(0);
      setPrefixes([]);
      setQualities([]);
      return undefined;
    }

    if (skipCategoryEffectRef.current === String(playlistId)) {
      skipCategoryEffectRef.current = '';
      return undefined;
    }

    let cancelled = false;
    setCategories([]);
    setCategoryId('');
    setAppliedSearch(null);
    setChannels([]);
    setTotal(0);
    setPrefixes([]);
    setQualities([]);
    setLoading(false);
    setError('');
    requestIdRef.current += 1;
    axios.get(`/api/playlists/${playlistId}/categories/active`)
      .then(({ data }) => {
        if (!cancelled) setCategories(data || []);
      })
      .catch(() => {
        if (!cancelled) setError('Unable to load categories.');
      });
    return () => { cancelled = true; };
  }, [playlistId]);

  const rememberQuickSearch = useCallback((term) => {
    setQuickSearches(current => {
      const existing = current.find(item => item.term.toLowerCase() === term.toLowerCase());
      const updated = [
        { term, uses: (existing?.uses || 0) + 1, lastUsed: Date.now() },
        ...current.filter(item => item.term.toLowerCase() !== term.toLowerCase()),
      ].sort((a, b) => b.uses - a.uses || b.lastUsed - a.lastUsed).slice(0, MAX_QUICK_SEARCHES);
      try {
        localStorage.setItem(QUICK_SEARCHES_KEY, JSON.stringify(updated));
      } catch (storageError) {
        console.warn('Unable to save channel search shortcut', storageError);
      }
      return updated;
    });
  }, []);

  const fetchResults = useCallback(async (filters, requestedPage, append = false) => {
    const requestId = ++requestIdRef.current;
    if (!filters?.playlistId || (!filters.categoryId && !filters.query.trim() && !filters.prefix && !filters.quality)) {
      setChannels([]);
      setTotal(0);
      setPrefixes([]);
      setQualities([]);
      setPage(0);
      setAppliedSearch(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError('');
    try {
      const params = {
        playlistId: filters.playlistId,
        page: requestedPage,
        pageSize: PAGE_SIZE,
      };
      if (filters.categoryId) params.categoryId = filters.categoryId;
      if (filters.query.trim()) params.q = filters.query.trim();
      if (filters.prefix) params.prefix = filters.prefix;
      if (filters.quality) params.quality = filters.quality;
      const { data } = await axios.get('/api/channels/search', { params });
      if (requestId !== requestIdRef.current) return;

      setChannels(current => append ? [...current, ...data.channels] : data.channels);
      setTotal(data.total);
      setPrefixes(data.prefixes || []);
      setQualities(data.qualities || []);
      setPage(data.page);
      setAppliedSearch(filters);
      if (!append && filters.query.trim() && !filters.prefix && !filters.quality && data.total > 0) {
        rememberQuickSearch(filters.query.trim());
      }
    } catch (requestError) {
      if (requestId === requestIdRef.current) {
        setError(requestError.response?.data?.error || 'Unable to search channels.');
      }
    } finally {
      if (requestId === requestIdRef.current) setLoading(false);
    }
  }, [rememberQuickSearch]);

  const submitSearch = (event) => {
    event?.preventDefault();
    fetchResults({ playlistId, categoryId, query, prefix: '', quality: '' }, 1);
  };

  const clearSearch = () => {
    setQuery('');
    if (appliedSearch?.query.trim()) {
      fetchResults({ ...appliedSearch, query: '' }, 1);
    }
  };

  const chooseCategory = (nextCategoryId) => {
    setCategoryId(nextCategoryId);
    fetchResults({ playlistId, categoryId: nextCategoryId, query, prefix: '', quality: '' }, 1);
  };

  const choosePlaylist = async (nextPlaylistId) => {
    if (String(nextPlaylistId) === String(playlistId)) return;

    const activeFilters = appliedSearch || { categoryId, query, prefix: '', quality: '' };
    const selectedCategory = categories.find(item => String(item.ID) === String(activeFilters.categoryId));
    const selectedCategoryName = selectedCategory?.CategoryName || selectedCategory?.category_name;
    const nextQuery = query;
    const requestId = ++playlistChangeRequestRef.current;

    skipCategoryEffectRef.current = String(nextPlaylistId);
    setPlaylistId(nextPlaylistId);
    setCategoryId('');
    setChannels([]);
    setTotal(0);
    setPrefixes([]);
    setQualities([]);
    setAppliedSearch(null);
    setLoading(true);
    setError('');
    requestIdRef.current += 1;

    try {
      const { data } = await axios.get(`/api/playlists/${nextPlaylistId}/categories/active`);
      if (requestId !== playlistChangeRequestRef.current) return;

      const nextCategories = data || [];
      setCategories(nextCategories);
      const matchingCategory = selectedCategoryName && nextCategories.find(item => {
        const name = item.CategoryName || item.category_name || '';
        return name.trim().toLowerCase() === selectedCategoryName.trim().toLowerCase();
      });
      const nextCategoryId = matchingCategory?.ID || '';
      setCategoryId(nextCategoryId);

      fetchResults({
        playlistId: nextPlaylistId,
        categoryId: nextCategoryId,
        query: nextQuery,
        prefix: activeFilters.prefix || '',
        quality: activeFilters.quality || '',
      }, 1);
    } catch (requestError) {
      if (requestId === playlistChangeRequestRef.current) {
        setLoading(false);
        setError(requestError.response?.data?.error || 'Unable to load categories.');
      }
    }
  };

  const toggleChannelFacet = (facetType, value) => {
    const current = appliedSearch || { playlistId, categoryId, query, prefix: '', quality: '' };
    const nextFilters = {
      ...current,
      [facetType]: current[facetType] === value ? '' : value,
    };
    fetchResults(nextFilters, 1);
  };

  const loadMore = () => {
    if (appliedSearch && !loading) {
      fetchResults(appliedSearch, page + 1, true);
    }
  };

  const getStreamUrl = channel => {
    const token = new URLSearchParams(window.location.search).get('secure');
    return `${window.location.origin}/hls/${channel.ID}.ts${token ? `?secure=${token}` : ''}`;
  };

  const copyStream = async channel => {
    try {
      await copyToClipboard(getStreamUrl(channel));
      openSnackbar('Stream link copied to clipboard');
    } catch (copyError) {
      console.error('Unable to copy channel stream link', copyError);
      openSnackbar('Unable to copy stream link');
    }
  };

  const playInVLC = channel => {
    const url = getStreamUrl(channel);
    window.location.href = createVLCIntentURL(url);
  };

  useEffect(() => {
    if (!openPlayer || !currentChannel || !mpegts.isSupported()) return undefined;
    const token = new URLSearchParams(window.location.search).get('secure');
    let player;
    const timeout = setTimeout(() => {
      if (!videoRef.current) return;
      player = mpegts.createPlayer({
        type: 'mpegts',
        isLive: true,
        url: `/hls/${currentChannel.ID}.ts?webbrowser=true${token ? `&secure=${token}` : ''}`,
      });
      player.attachMediaElement(videoRef.current);
      player.load();
      player.play().catch(error => console.warn('Unable to start channel preview', error));
    }, 100);

    return () => {
      clearTimeout(timeout);
      if (player) {
        player.detachMediaElement();
        player.destroy();
      }
    };
  }, [openPlayer, currentChannel]);

  const selectedPlaylist = playlists.find(item => String(item.ID) === String(playlistId));
  const noQualityFacet = qualities.find(facet => facet.value === 'NONE');

  return (
    <Container maxWidth="xl">
      <Box sx={{ py: 3 }}>
        <Typography variant="h4" fontWeight="bold" gutterBottom>Channel Explorer</Typography>

        <Stack spacing={2} sx={{ pb: 4 }}>
        <Box sx={{
          display: 'grid',
          gridTemplateColumns: { xs: 'minmax(0, 1fr)', md: 'repeat(3, minmax(0, 1fr))' },
          gap: { xs: '16px', md: '24px' },
        }}>
          <Box>
            <FormControl fullWidth>
              <InputLabel id="explorer-playlist-label">Playlist</InputLabel>
              <Select
                labelId="explorer-playlist-label"
                label="Playlist"
                value={playlistId}
                onChange={event => choosePlaylist(event.target.value)}
              >
                {playlists.map(item => (
                  <MenuItem key={item.ID} value={item.ID}>{item.Description || `Playlist ${item.ID}`}</MenuItem>
                ))}
              </Select>
            </FormControl>
          </Box>
          <Box>
            <FormControl fullWidth disabled={!playlistId}>
              <InputLabel id="explorer-category-label">Category (optional)</InputLabel>
              <Select
                labelId="explorer-category-label"
                label="Category (optional)"
                value={categoryId}
                onChange={event => chooseCategory(event.target.value)}
              >
                <MenuItem value="">All categories</MenuItem>
                {categories.map(item => (
                  <MenuItem key={item.ID} value={item.ID}>{item.CategoryName || item.category_name}</MenuItem>
                ))}
              </Select>
            </FormControl>
          </Box>
          <Box>
            <Box component="form" onSubmit={submitSearch}>
              <TextField
                fullWidth
                label="Search channels"
                placeholder="e.g. news, sports..."
                value={query}
                onChange={event => setQuery(event.target.value)}
                disabled={!playlistId}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      {(query || appliedSearch?.query) && (
                        <IconButton
                          type="button"
                          aria-label="Clear search"
                          onClick={clearSearch}
                          disabled={!playlistId || loading}
                        >
                          <ClearIcon />
                        </IconButton>
                      )}
                      <IconButton
                        type="submit"
                        aria-label="Search"
                        edge="end"
                        disabled={!playlistId || loading}
                      >
                        <SearchIcon />
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />
            </Box>
          </Box>
        </Box>

        {quickSearches.length > 0 && (
          <Box>
            <Box sx={{ display: 'flex', flexWrap: 'wrap', columnGap: 3, rowGap: 1 }}>
              {quickSearches.map(item => (
                <Chip
                  key={item.term}
                  label={`${item.term.toUpperCase()} · ${item.uses}`}
                  onClick={() => {
                    setQuery(item.term);
                    fetchResults({ playlistId, categoryId, query: item.term, prefix: '', quality: '' }, 1);
                  }}
                  disabled={!playlistId || loading}
                  color="primary"
                  variant="outlined"
                />
              ))}
            </Box>
          </Box>
        )}

        {error && <Alert severity="error">{error}</Alert>}

        {appliedSearch && (
          <Box sx={{ display: 'flex', flexDirection: 'column' }}>
            {prefixes.length > 0 && (
              <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 3, rowGap: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 72 }}>Provider</Typography>
                {prefixes.map(facet => (
                  <Chip
                    key={facet.value}
                    label={`${facet.value} (${facet.count})`}
                    onClick={() => toggleChannelFacet('prefix', facet.value)}
                    color={appliedSearch.prefix === facet.value ? 'primary' : 'default'}
                    variant={appliedSearch.prefix === facet.value ? 'filled' : 'outlined'}
                    disabled={loading}
                  />
                ))}
              </Box>
            )}
            {prefixes.length > 0 && appliedSearch && (
              <Box sx={{ height: 8, flex: '0 0 8px' }} />
            )}
            {appliedSearch && (
              <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 3, rowGap: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ minWidth: 72 }}>Quality</Typography>
                {qualities.filter(facet => facet.value !== 'NONE').map(facet => (
                  <Chip
                    key={facet.value}
                    label={`${facet.value} (${facet.count})`}
                    onClick={() => toggleChannelFacet('quality', facet.value)}
                    color={appliedSearch.quality === facet.value ? 'primary' : 'default'}
                    variant={appliedSearch.quality === facet.value ? 'filled' : 'outlined'}
                    disabled={loading}
                  />
                ))}
                <Chip
                  label={`No quality detected${noQualityFacet ? ` (${noQualityFacet.count})` : ''}`}
                  onClick={() => toggleChannelFacet('quality', 'NONE')}
                  color={appliedSearch.quality === 'NONE' ? 'primary' : 'default'}
                  variant={appliedSearch.quality === 'NONE' ? 'filled' : 'outlined'}
                  disabled={loading}
                />
              </Box>
            )}
          </Box>
        )}

        {!appliedSearch && !loading && (
          <Alert severity="info">
            {selectedPlaylist
              ? 'Enter a name to search the playlist or select a category to browse its channels.'
              : 'Add a playlist to start exploring channels.'}
          </Alert>
        )}

        </Stack>

        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
          {appliedSearch && (
            <Box sx={{ height: 8, flex: '0 0 8px' }} />
          )}
          {appliedSearch && (
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Typography color="text.secondary">
                {total.toLocaleString()} results · showing {channels.length.toLocaleString()}
              </Typography>
              {loading && <CircularProgress size={22} />}
            </Box>
          )}
          {appliedSearch && (
            <Box sx={{ height: 8, flex: '0 0 8px' }} />
          )}

          {channels.length > 0 && (
          <Grid container spacing={2} sx={{ mt: 0 }}>
            {channels.map(channel => {
              const presentation = getChannelPresentation(channel);
              return (
              <Grid item xs={12} sm={6} md={4} lg={3} xl={2} key={channel.ID}>
                <Card sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                  <ChannelArtwork channel={channel} prefix={presentation.prefix} quality={presentation.quality} />
                  <CardContent sx={{ flexGrow: 1, pb: 1 }}>
                    <Typography
                      variant="subtitle1"
                      fontWeight="bold"
                      title={presentation.name}
                      sx={{
                        display: '-webkit-box',
                        WebkitBoxOrient: 'vertical',
                        WebkitLineClamp: 2,
                        overflow: 'hidden',
                      }}
                    >
                      {presentation.name}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" noWrap display="block">
                      {channel.Category?.category_name || channel.Category?.CategoryName || 'Uncategorized'}
                    </Typography>
                  </CardContent>
                  <CardActions sx={{ justifyContent: 'space-around', pt: 0 }}>
                    <Tooltip title="Play">
                      <IconButton color="primary" aria-label={`Play ${channel.name}`} onClick={() => {
                        setCurrentChannel(channel);
                        setOpenPlayer(true);
                      }}>
                        <LiveTvIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Open in VLC">
                      <IconButton aria-label={`Open ${channel.name} in VLC`} onClick={() => playInVLC(channel)}>
                        <VLCIcon />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy link">
                      <IconButton aria-label={`Copy link for ${channel.name}`} onClick={() => copyStream(channel)}>
                        <ContentCopyIcon />
                      </IconButton>
                    </Tooltip>
                  </CardActions>
                </Card>
              </Grid>
              );
            })}
          </Grid>
          )}

          {appliedSearch && total === 0 && !loading && (
            <Alert severity="info">No channels found for these filters.</Alert>
          )}
          {channels.length < total && (
            <>
            {channels.length > 0 && <Box sx={{ height: 24, flex: '0 0 24px' }} />}
            <Box sx={{ display: 'flex', justifyContent: 'center' }}>
              <Button variant="outlined" onClick={loadMore} disabled={loading}>
                {loading ? 'Loading...' : 'Load more channels'}
              </Button>
            </Box>
            </>
          )}
        </Box>
      </Box>

      <Dialog open={openPlayer} onClose={() => setOpenPlayer(false)} maxWidth="md" fullWidth>
        <DialogTitle>{currentChannel?.name}</DialogTitle>
        <DialogContent>
          <Box sx={{ width: '100%', aspectRatio: '16 / 9', bgcolor: 'black' }}>
            <video ref={videoRef} controls autoPlay style={{ width: '100%', height: '100%' }} />
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenPlayer(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default ChannelExplorer;
