import React, { useEffect, useContext,useState,  useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Grid, Container, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, IconButton, Box, Button, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Paper, Tooltip, Typography } from '@mui/material';
import { SnackbarContext } from './SnackbarContext';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import PendingIcon from '@mui/icons-material/Pending';
import InProgressIcon from '@mui/icons-material/Update';
import AcceptIcon from '@mui/icons-material/Check';
import DeclineIcon from '@mui/icons-material/Clear';
import QuestionMarkIcon from '@mui/icons-material/QuestionMark';
import Fab from '@mui/material/Fab';
import AddIcon from '@mui/icons-material/Add';
import { useTheme } from '@mui/material/styles';
import useMediaQuery from '@mui/material/useMediaQuery';
import axios from 'axios';


const DeleteButton = ({ playlistId, onDeletion, onSnackbarOpen }) => {
  const [open, setOpen] = useState(false);

  const handleClickOpen = () => {
    setOpen(true);
  };

  const handleClose = () => {
    setOpen(false);
  };

  const handleDelete = async () => {
    try {
      await axios.delete(`/api/playlist/${playlistId}`);
      onSnackbarOpen('Playlist successfully deleted!');
      onDeletion();
    } catch (err) {
      alert('Failed to delete the playlist');
    }
    setOpen(false);
  };

  return (
    <>
      <IconButton color="primary" onClick={handleClickOpen}>
        <DeleteIcon />
      </IconButton>
      <Dialog
        open={open}
        onClose={handleClose}
        aria-labelledby="alert-dialog-title"
        aria-describedby="alert-dialog-description"
      >
        <DialogTitle id="alert-dialog-title">{"Delete Playlist"}</DialogTitle>
        <DialogContent>
          <DialogContentText id="alert-dialog-description">
            Are you sure you want to delete this playlist?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose}>Cancel</Button>
          <Button onClick={handleDelete} autoFocus>
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

const getIconByStatus = (status) => {
  switch (status) {
    case 0: return <PendingIcon />;
    case 1: return <InProgressIcon />;
    case 2: return <AcceptIcon />;
    case -1: return <DeclineIcon />;
    default: return <QuestionMarkIcon />;
  }
};

const getTooltipByStatus = (status) => {
  switch (status) {
    case 0: return "Import not started - click to start";
    case 1: return "Import in progress";
    case 2: return "Import completed - click to retrigger";
    case -1: return "Import failed - click to retry";
    default: return "Unknown status";
  }
};

const ImportStatusButton = ({ playlist, importStatus }) => {
  const navigate = useNavigate();
  const handleImport = async () => {
    if (playlist.ImportStatus !== 1) {
      axios.put(`/api/playlist/${playlist.ID}`, playlist);
      navigate('/playlists');
    }
  };

  return (
    <Tooltip title={getTooltipByStatus(importStatus || playlist.ImportStatus)}>
      <IconButton color="primary" onClick={handleImport}>
        {getIconByStatus(importStatus || playlist.ImportStatus)}
      </IconButton>
    </Tooltip>
  );
};

const EPGStatusButton = ({ epgStatus, handlePress }) => {
  return (
    <Tooltip title={getTooltipByStatus(epgStatus)}>
      <IconButton color="primary" onClick={handlePress}>
        {getIconByStatus(epgStatus)}
      </IconButton>
    </Tooltip>
  );
};

const PlaylistTable = () => {
  const [rows, setRows] = useState([]);
  const navigate = useNavigate();

  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  const fetchPlaylists = async () => {
    const response = await fetch('/api/playlists');
    return response.json();
  };

  const refreshPlaylists = useCallback(async () => {
    let playlists = await fetchPlaylists();
    setRows(playlists);

    let isImporting = playlists.some(playlist => {
      if ([0, 1].includes(playlist.ImportStatus)) {
        return true; // Continue checking if ImportStatus is 0 or 1
      } else if (playlist.ImportStatus === 2) {
        return [0, 1].includes(playlist.EpgStatus); // Continue checking if ImportStatus is 2 and EpgStatus is 0 or 1
      }
      return false; 
    });    
    
    if (isImporting) {
      setTimeout(refreshPlaylists, 2000);
    }
  }, []);

  useEffect(() => {
    refreshPlaylists();
  }, [refreshPlaylists]);

  const secondsToDate = (secondsStr) => {
      const seconds = parseInt(secondsStr, 10);
      const date = new Date(seconds * 1000);
      const day = date.getDate();
      const month = date.getMonth() + 1;
      const year = date.getFullYear();
      const formattedDate = `${day.toString().padStart(2, '0')}/${month.toString().padStart(2, '0')}/${year}`;
      return formattedDate;
  }

  const handleDeletion = (message) => {
    openSnackbar(message);
  };

  const handleEdit = (playlistId) => {
    navigate(`/playlists/${playlistId}`);
  };

  const handleAddNew = () => {
    navigate("/playlists/new");
  };

  const { openSnackbar } = useContext(SnackbarContext);

  return (
    <Container>
    <Box>
      <Typography variant="h5" sx={{ marginBottom: 10, fontWeight: 'bold' }}>Playlists</Typography>
      {isMobile ? (
        <Grid container spacing={2} >
          {rows.map((row) => (
            <Grid item xs={12} key={row.ID} >
              <Paper
                sx={{
                  padding: 2,
                  display: 'flex',
                  flexDirection: 'column',
                  marginBottom: 1,
                }}
              >
                <Grid container spacing={1} sx={{ margin: [0, 0, 0, 0] }}>
                  <Grid item xs={12}>{row.ID}: {row.Description}{row.Expired ? " (Expired)" : ""}</Grid>
                  <Grid item xs={6}>Type: {row.Type}</Grid>
                  <Grid item xs={12}>Import Status: <ImportStatusButton importStatus={row.ImportStatus} /></Grid>
                  <Grid item xs={12}>EPG Status: <EPGStatusButton epgStatus={row.EpgStatus} /></Grid>
                  <Grid item xs={12} sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                    <IconButton color="primary" onClick={() => handleEdit(row.ID)}>
                      <EditIcon />
                    </IconButton>
                    <DeleteButton playlistId={row.ID} onDeletion={refreshPlaylists} onSnackbarOpen={handleDeletion} />
                  </Grid>
                </Grid>
              </Paper>
            </Grid>
          ))}
        </Grid>
      ) : (
      <TableContainer component={Paper}>
        <Table >
          <TableHead>
            <TableRow>
              <TableCell>ID</TableCell>
              <TableCell>Description</TableCell>
              <TableCell>Type</TableCell>
              <TableCell>Expiration</TableCell>
              <TableCell>Actions</TableCell>
              <TableCell>Import Status</TableCell>
              <TableCell>EPG Status</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.ID}>
                <TableCell>{row.ID}</TableCell>
                <TableCell>{row.Description}</TableCell>
                <TableCell>{row.Type}</TableCell>
                <TableCell>{row.Expired ? "Expired" : secondsToDate(row.ExpiresAt)}</TableCell>
                <TableCell>
                  <IconButton color="primary" onClick={() => handleEdit(row.ID)}>
                    <EditIcon />
                  </IconButton>
                  <DeleteButton playlistId={row.ID} onDeletion={refreshPlaylists} onSnackbarOpen={handleDeletion} />
                </TableCell>
                <TableCell>
                  <ImportStatusButton playlist={row} importStatus={row.importStatus} />
                </TableCell>
                <TableCell>
                  <EPGStatusButton epgStatus={row.EpgStatus} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <Fab
          color="primary"
          aria-label="Add New Playlist"
          sx={{ position: 'fixed', bottom: 16, right: 16 }}
          onClick={handleAddNew}
        >
          <AddIcon />
        </Fab>
      </TableContainer>
      )}
    </Box>
    </Container>
  );
};

export default PlaylistTable;
