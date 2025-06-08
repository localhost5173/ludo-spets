// State constants
const STATE = {
  SELECT_GAME: 0,
  TIME_SELECT: 1,
  PAYMENT: 2,
  EXTEND_TIME: 3,
  EXTEND_PAYMENT: 4,
  GAME_LOADING: 5, // Add new loading state
  GAME_ACTIVE: 6, // Add new active game state
};

// Application state
let appState = {
  currentState: STATE.SELECT_GAME,
  selectedGameIndex: 0,
  selectedGameName: "",
  games: [],
  timeValue: 5,
  pricePerMinute: 0.5,
};

// WebSocket connection
let socket;

// Initialize the application
document.addEventListener("DOMContentLoaded", () => {
  initWebSocket();
  loadGames();
  setupEventListeners();

  // Add a slight delay to ensure browser is ready
  setTimeout(() => {
    try {
      requestFullscreen();
    } catch (e) {
      console.error("Couldn't enter fullscreen mode:", e);
    }
  }, 1000);
});

// Initialize WebSocket connection
function initWebSocket() {
  // Get the correct WebSocket URL based on the current page URL
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsUrl = `${protocol}//${window.location.host}/ws`;

  socket = new WebSocket(wsUrl);

  socket.onopen = () => {
    console.log("WebSocket connection established");
  };

  socket.onmessage = (event) => {
    const message = JSON.parse(event.data);
    handleWebSocketMessage(message);
  };

  socket.onclose = () => {
    console.log("WebSocket connection closed");
    // Attempt to reconnect after a delay
    setTimeout(initWebSocket, 5000);
  };

  socket.onerror = (error) => {
    console.error("WebSocket error:", error);
  };
}

// Handle incoming WebSocket messages
function handleWebSocketMessage(message) {
  console.log("Received message:", message);

  switch (message.type) {
    case "state":
      console.log("State change received:", message.payload);
      updateUIState(message.payload);
      break;

    case "window_position":
      positionOverGame(message.payload);
      break;

    case "game_loading":
      showGameLoadingMessage(message.payload.message);
      break;

    case "game_started":
      // Handle game started message - update to active state
      console.log("Game started:", message.payload.message);
      appState.currentState = STATE.GAME_ACTIVE;
      updateUIState(STATE.GAME_ACTIVE);
      break;

    case "prepare_timeout":
      // Game will timeout soon, prepare UI
      console.log("Preparing for timeout:", message.payload.message);
      break;

    default:
      console.log("Unknown message type:", message.type);
  }
}

// Show game loading message
function showGameLoadingMessage(message) {
  const statusText = document.getElementById("status-text");
  statusText.textContent = message;
  
  // Hide all other UI elements
  const gameGrid = document.getElementById("game-grid");
  const timeSelection = document.getElementById("time-selection");
  const paymentPrompt = document.getElementById("payment-prompt");
  
  gameGrid.classList.add("hidden");
  timeSelection.classList.add("hidden");
  paymentPrompt.classList.add("hidden");
}

// Position the browser window over the game window
function positionOverGame(windowInfo) {
  if (appState.currentState !== STATE.EXTEND_TIME) {
    return; // Only reposition during timeout
  }

  console.log("Positioning browser over game:", windowInfo);

  // Set a unique window title that we can search for in OS commands
  document.title = "SPETS ARCADE - TIMEOUT OVERLAY";
  
  // The timeout overlay should already be showing from updateUIState
  // This function just handles any additional positioning logic if needed
}

// Handle keypresses during timeout overlay
function handleTimeoutKeypress(event) {
  console.log("Key pressed in timeout overlay:", event.key, "Current state:", appState.currentState);
  
  if (appState.currentState !== STATE.EXTEND_TIME && appState.currentState !== STATE.EXTEND_PAYMENT) {
    console.log("Not in extend time or payment state, removing key handler");
    document.removeEventListener("keydown", handleTimeoutKeypress);
    return;
  }

  // Bring window to focus with each keypress
  window.focus();

  // Handle keys based on current state
  if (appState.currentState === STATE.EXTEND_TIME) {
    // Time selection handling during timeout
    switch (event.key) {
      case "ArrowLeft":
        if (appState.timeValue > 1) {
          appState.timeValue--;
          updateTimeSliderInOverlay();
          updatePrice();
        }
        break;
        
      case "ArrowRight":
        if (appState.timeValue < 60) {
          appState.timeValue++;
          updateTimeSliderInOverlay();
          updatePrice();
        }
        break;
        
      case "Enter":
        // Move to payment step
        const instructions = document.getElementById("timeout-instructions");
        if (instructions) {
          instructions.textContent = 'Step 2: Press "P" to pay and resume your game';
        }
        
        // Send message to confirm time selection
        sendMessage("selectTime", appState.timeValue);
        
        // State will be updated via WebSocket message
        break;
        
      case "Escape":
        // Handle quit - go back to game selection
        handleQuitGame();
        break;
    }
  } else if (appState.currentState === STATE.EXTEND_PAYMENT) {
    // Payment handling during timeout
    console.log("In EXTEND_PAYMENT state, key pressed:", event.key);
    
    switch (event.key) {
      case "p":
      case "P":
        console.log("P key detected in payment state, calling handleExtendPayment()");
        handleExtendPayment();
        break;
        
      case "Escape":
        console.log("ESC key detected, going back to time selection");
        appState.currentState = STATE.EXTEND_TIME;
        updateUIState(STATE.EXTEND_TIME);
        break;
    }
  }
}

// Update the time slider in the overlay
function updateTimeSliderInOverlay() {
  const overlay = document.getElementById("game-overlay");
  if (overlay) {
    const slider = overlay.querySelector("#time-slider");
    if (slider) {
      slider.value = appState.timeValue;
    }
  }
  
  // Also update the original slider
  const originalSlider = document.getElementById("time-slider");
  if (originalSlider) {
    originalSlider.value = appState.timeValue;
  }
  
  // Update the price displays in both places
  updatePrice();
  updateOverlayPrice();
}

// Update price in the overlay
function updateOverlayPrice() {
  const overlay = document.getElementById("game-overlay");
  if (!overlay) return;
  
  const priceLabel = overlay.querySelector("#price-label");
  if (priceLabel) {
    const price = appState.timeValue * appState.pricePerMinute;
    priceLabel.textContent = `TOTAL COST: $${price.toFixed(2)} (${appState.timeValue} minutes)`;
  }
}

// Handle extend payment - browser stays open, just update UI
function handleExtendPayment() {
  console.log("handleExtendPayment() called - Processing payment for time extension");
  
  // Update status immediately
  const instructions = document.getElementById("timeout-instructions");
  if (instructions) {
    instructions.textContent = 'PROCESSING PAYMENT... RESUMING GAME...';
  }

  // Send payment message
  try {
    const payloadData = {
      gameName: appState.selectedGameName,
      minutes: appState.timeValue,
    };
    
    const messageJSON = JSON.stringify({
      type: "payment",
      payload: payloadData
    });
    console.log("Sending payment JSON:", messageJSON);
    
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(messageJSON);
    } else {
      console.error("WebSocket not open, state:", socket ? socket.readyState : "null");
    }
  } catch (e) {
    console.error("Error sending payment message:", e);
  }

  // Show processing state for a moment, then update UI
  setTimeout(() => {
    const overlay = document.getElementById("game-overlay");
    if (overlay) {
      overlay.style.display = "none";
    }
    
    // Update state to show game is active
    appState.currentState = STATE.GAME_ACTIVE;
    updateUIState(STATE.GAME_ACTIVE);
    
    console.log("Payment processed, game should resume - browser stays open");
  }, 500);
}

// Handle quit game
function handleQuitGame() {
  console.log("Player chose to quit game");
  
  // Hide overlay
  const overlay = document.getElementById("game-overlay");
  if (overlay) {
    overlay.style.display = "none";
  }

  // Send quit message
  sendMessage("quit", {});

  // Remove timeout key handler
  document.removeEventListener("keydown", handleTimeoutKeypress);

  // Update UI state will be handled via WebSocket message
}

// Show the timeout overlay immediately
function showTimeoutOverlay() {
  console.log("Showing timeout overlay - clearing any existing state");

  // Force focus and bring window to front
  window.focus();

  // Create overlay container if it doesn't exist
  let overlay = document.getElementById("game-overlay");
  if (!overlay) {
    overlay = document.createElement("div");
    overlay.id = "game-overlay";
    document.body.appendChild(overlay);
  }

  // Clear existing content and force display
  overlay.innerHTML = "";
  overlay.style.display = "flex"; // Force display
  
  // Add title tag to help window managers identify this window
  document.title = "SPETS ARCADE - TIMEOUT OVERLAY";

  // Style overlay with high z-index and forced visibility
  overlay.style.position = "fixed";
  overlay.style.left = "0";
  overlay.style.top = "0";
  overlay.style.width = "100%";
  overlay.style.height = "100%";
  overlay.style.backgroundColor = "rgba(0, 0, 0, 0.95)";
  overlay.style.display = "flex";
  overlay.style.flexDirection = "column";
  overlay.style.justifyContent = "center";
  overlay.style.alignItems = "center";
  overlay.style.zIndex = "999999";
  overlay.style.visibility = "visible"; // Force visibility
  
  // Add pulsing animation for visibility
  if (!document.getElementById('pulse-animation')) {
    const style = document.createElement('style');
    style.id = 'pulse-animation';
    style.textContent = `
      @keyframes pulse {
        0% { transform: scale(1); }
        50% { transform: scale(1.05); }
        100% { transform: scale(1); }
      }
    `;
    document.head.appendChild(style);
  }

  // Create timeout message
  const timeoutMsg = document.createElement("h1");
  timeoutMsg.id = "timeout-message";
  timeoutMsg.textContent = "TIME OUT!";
  timeoutMsg.style.color = "#cf2e2e";
  timeoutMsg.style.fontSize = "4rem";
  timeoutMsg.style.marginBottom = "2rem";
  timeoutMsg.style.textShadow = "0 0 20px rgba(255, 0, 0, 0.7)";
  timeoutMsg.style.animation = "pulse 1s infinite ease-in-out";
  overlay.appendChild(timeoutMsg);

  // Create instructions
  const instructions = document.createElement("p");
  instructions.id = "timeout-instructions";
  instructions.textContent = 'Step 1: Select time with ◄ ► and press ENTER to continue';
  instructions.style.color = "#fecda5";
  instructions.style.fontSize = "1.5rem";
  instructions.style.marginBottom = "2rem";
  overlay.appendChild(instructions);

  // Clone the time selection UI into the overlay
  const originalTimeSelection = document.getElementById("time-selection");
  const timeSelectionClone = originalTimeSelection.cloneNode(true);
  timeSelectionClone.style.position = "relative";
  timeSelectionClone.style.zIndex = "1001";
  timeSelectionClone.style.display = "block";
  overlay.appendChild(timeSelectionClone);
  
  // Add event listener to the cloned slider
  const clonedSlider = timeSelectionClone.querySelector("#time-slider");
  if (clonedSlider) {
    console.log("Setting up event listener for cloned slider");
    clonedSlider.value = appState.timeValue;
    
    clonedSlider.addEventListener("input", () => {
      console.log("Cloned slider value changed to:", clonedSlider.value);
      appState.timeValue = parseInt(clonedSlider.value);
      updatePrice();
      updateOverlayPrice();
    });
  }

  // Add quit option
  const quitInstructions = document.createElement("p");
  quitInstructions.textContent = 'Press ESC to quit and return to game selection';
  quitInstructions.style.color = "#888";
  quitInstructions.style.fontSize = "1rem";
  quitInstructions.style.marginTop = "2rem";
  overlay.appendChild(quitInstructions);

  // Add key handlers
  document.removeEventListener("keydown", handleTimeoutKeypress);
  document.addEventListener("keydown", handleTimeoutKeypress);
  console.log("Added keydown event listener for timeout overlay");

  // Global P key handler
  const globalPKeyHandler = function(event) {
    if ((event.key === 'p' || event.key === 'P') && 
        (appState.currentState === STATE.EXTEND_PAYMENT || appState.currentState === STATE.EXTEND_TIME)) {
      console.log("P key detected from global key handler - processing payment");
      handleExtendPayment();
    }
  };
  
  document.removeEventListener("keypress", globalPKeyHandler);
  document.removeEventListener("keydown", globalPKeyHandler);
  document.addEventListener("keypress", globalPKeyHandler);
  document.addEventListener("keydown", globalPKeyHandler);
  window.globalPKeyHandler = globalPKeyHandler;

  // Force focus and visibility
  window.focus();
  
  // Continuous focus attempts with visual feedback
  let focusAttempts = 0;
  const focusInterval = setInterval(() => {
    window.focus();
    
    // Flash the overlay
    if (overlay) {
      const originalBg = overlay.style.backgroundColor;
      overlay.style.backgroundColor = "rgba(255, 0, 0, 0.8)";
      setTimeout(() => {
        if (overlay) overlay.style.backgroundColor = originalBg;
      }, 100);
    }
    
    focusAttempts++;
    if (focusAttempts > 40) {
      clearInterval(focusInterval);
    }
  }, 500);
  
  // Force fullscreen
  requestFullscreen();
  
  console.log("Timeout overlay should now be visible");
}

// Update the UI based on the current state
function updateUIState(newState) {
  console.log("Updating UI state from", appState.currentState, "to", newState);
  
  // If transitioning from EXTEND_TIME to EXTEND_PAYMENT, send an extra selectTime message
  if (appState.currentState === STATE.EXTEND_TIME && newState === STATE.EXTEND_PAYMENT) {
    console.log("Critical state transition: Sending additional selectTime message");
    sendMessage("selectTime", appState.timeValue);
  }
  
  // Update current state
  appState.currentState = newState;

  const gameGrid = document.getElementById("game-grid");
  const timeSelection = document.getElementById("time-selection");
  const paymentPrompt = document.getElementById("payment-prompt");
  const statusText = document.getElementById("status-text");

  // Hide all sections first
  gameGrid.classList.add("hidden");
  timeSelection.classList.add("hidden");
  paymentPrompt.classList.add("hidden");

  // Remove overlay if present and not in timeout states
  if (newState !== STATE.EXTEND_TIME && newState !== STATE.EXTEND_PAYMENT) {
    console.log("State not extend_time or extend_payment, removing overlay");
    const overlay = document.getElementById("game-overlay");
    if (overlay) {
      overlay.style.display = "none";
    }
    document.removeEventListener("keydown", handleTimeoutKeypress);
  } else {
    console.log("In extend_time or extend_payment state, keeping overlay and key handler");
  }

  // Show appropriate section based on state
  switch (newState) {
    case STATE.SELECT_GAME:
      statusText.textContent = "◄ ► ▲ ▼ NAVIGATE    ENTER TO CONTINUE";
      gameGrid.classList.remove("hidden");
      break;

    case STATE.TIME_SELECT:
      statusText.textContent = "◄ ► ADJUST TIME    ENTER TO CONTINUE";
      timeSelection.classList.remove("hidden");
      updatePrice();
      break;

    case STATE.PAYMENT:
      statusText.textContent = "PRESS 'P' TO INSERT COIN AND START GAME";
      paymentPrompt.classList.remove("hidden");
      break;

    case STATE.EXTEND_TIME:
      console.log("Entering EXTEND_TIME state - showing timeout overlay");
      statusText.textContent = "TIME OUT! Select additional time and press ENTER";
      
      // Force the overlay to show regardless of current UI state
      showTimeoutOverlay();
      break;

    case STATE.EXTEND_PAYMENT:
      console.log("Entered EXTEND_PAYMENT state, setting up payment UI");
      statusText.textContent = "Step 2: PRESS 'P' TO INSERT COIN AND RESUME GAME";
      
      // Update overlay instructions if it exists
      const instructions = document.getElementById("timeout-instructions");
      if (instructions) {
        instructions.textContent = 'Step 2: Press "P" to pay and resume your game';
      }
      
      window.focus();
      break;

    case STATE.GAME_LOADING:
      console.log("Game loading state");
      statusText.textContent = "STARTING GAME... PLEASE WAIT";
      break;

    case STATE.GAME_ACTIVE:
      console.log("Game active state - hiding all UI elements");
      statusText.textContent = "GAME IN PROGRESS...";
      // Ensure all UI elements are hidden during active game
      const overlay = document.getElementById("game-overlay");
      if (overlay) {
        overlay.style.display = "none";
      }
      break;
  }
}

// Send a message through the WebSocket
function sendMessage(type, payload) {
  if (socket.readyState === WebSocket.OPEN) {
    socket.send(
      JSON.stringify({
        type: type,
        payload: payload,
      })
    );
  } else {
    console.error("WebSocket is not connected");
  }
}

// Load games from the API
function loadGames() {
  fetch("/api/games")
    .then((response) => response.json())
    .then((games) => {
      appState.games = games;
      renderGameGrid();
    })
    .catch((error) => {
      console.error("Error loading games:", error);
    });
}

// Render the game grid
function renderGameGrid() {
  const gameGrid = document.getElementById("game-grid");
  gameGrid.innerHTML = "";

  // Calculate grid size
  const cols = 8; // Always use 8 columns as in Fyne UI

  appState.games.forEach((game, index) => {
    const tile = document.createElement("div");
    tile.className = "game-tile";
    tile.setAttribute("tabindex", "0");
    tile.dataset.index = index;

    if (index === appState.selectedGameIndex) {
      tile.classList.add("selected");
      // Set the selectedGameName when rendering the grid initially
      appState.selectedGameName = game.name;
    }

    const imgContainer = document.createElement("div");
    imgContainer.className = "game-tile-image";

    const img = document.createElement("img");
    img.src = game.imagePath || "/assets/spets/games/default.png";
    img.alt = game.name;

    const nameDiv = document.createElement("div");
    nameDiv.className = "game-tile-name";
    nameDiv.textContent = game.name;

    imgContainer.appendChild(img);
    tile.appendChild(imgContainer);
    tile.appendChild(nameDiv);

    tile.addEventListener("click", () => {
      selectGameTile(index);
    });

    gameGrid.appendChild(tile);
  });

  // Make sure the selected tile is visible
  focusSelectedTile();
}

// Select a game tile
function selectGameTile(index) {
  if (appState.currentState !== STATE.SELECT_GAME) return;

  // Update selected index
  appState.selectedGameIndex = index;
  appState.selectedGameName = appState.games[index].name;

  // Update UI
  const tiles = document.querySelectorAll(".game-tile");
  tiles.forEach((tile, i) => {
    if (i === index) {
      tile.classList.add("selected");
    } else {
      tile.classList.remove("selected");
    }
  });
}

// Ensure the selected tile is visible in the viewport with better scrolling
function focusSelectedTile() {
  const selectedTile = document.querySelector(".game-tile.selected");
  if (selectedTile) {
    // Calculate position for centered scrolling
    const container = document.querySelector(".game-grid-container");
    const tileRect = selectedTile.getBoundingClientRect();
    const containerRect = container.getBoundingClientRect();

    // Calculate scroll position to center the selected tile
    const scrollTop =
      selectedTile.offsetTop - containerRect.height / 2 + tileRect.height / 2;
    const scrollLeft =
      selectedTile.offsetLeft - containerRect.width / 2 + tileRect.width / 2;

    // Apply scroll with smooth behavior
    container.scrollTo({
      top: Math.max(0, scrollTop),
      left: Math.max(0, scrollLeft),
      behavior: "smooth",
    });
  }
}

// Update the price display based on the time value
function updatePrice() {
  const priceLabel = document.getElementById("price-label");
  if (!priceLabel) return;
  
  const price = appState.timeValue * appState.pricePerMinute;
  priceLabel.textContent = `TOTAL COST: $${price.toFixed(2)} (${appState.timeValue} minutes)`;
  
  // Also update the overlay price
  updateOverlayPrice();
}

// Setup event listeners for keyboard navigation and interaction
function setupEventListeners() {
  // Time slider input event
  const timeSlider = document.getElementById("time-slider");
  timeSlider.addEventListener("input", () => {
    appState.timeValue = parseInt(timeSlider.value);
    updatePrice();
  });

  // Keyboard navigation
  document.addEventListener("keydown", (event) => {
    // Don't handle keys if timeout overlay is active - that has its own handler
    if (appState.currentState === STATE.EXTEND_TIME || appState.currentState === STATE.EXTEND_PAYMENT) {
      return;
    }
    
    switch (appState.currentState) {
      case STATE.SELECT_GAME:
        handleGameSelectionKeys(event);
        break;

      case STATE.TIME_SELECT:
        handleTimeSelectionKeys(event);
        break;

      case STATE.PAYMENT:
        handlePaymentKeys(event);
        break;
    }
  });
}

// Handle keyboard navigation in game selection state
function handleGameSelectionKeys(event) {
  const cols = 8; // Always use 8 columns
  const numGames = appState.games.length;
  let newIndex = appState.selectedGameIndex;

  switch (event.key) {
    case "ArrowDown":
      newIndex += cols;
      if (newIndex < numGames) {
        selectGameTile(newIndex);
        focusSelectedTile();
      }
      break;

    case "ArrowUp":
      newIndex -= cols;
      if (newIndex >= 0) {
        selectGameTile(newIndex);
        focusSelectedTile();
      }
      break;

    case "ArrowLeft":
      if (newIndex > 0) {
        selectGameTile(newIndex - 1);
        focusSelectedTile();
      }
      break;

    case "ArrowRight":
      if (newIndex < numGames - 1) {
        selectGameTile(newIndex + 1);
        focusSelectedTile();
      }
      break;

    case "Enter":
      sendMessage("selectGame", appState.selectedGameName);
      break;
  }
}

// Handle keyboard navigation in time selection state
function handleTimeSelectionKeys(event) {
  const timeSlider = document.getElementById("time-slider");

  switch (event.key) {
    case "ArrowLeft":
      if (appState.timeValue > 1) {
        appState.timeValue--;
        timeSlider.value = appState.timeValue;
        updatePrice();
      }
      break;

    case "ArrowRight":
      if (appState.timeValue < 60) {
        appState.timeValue++;
        timeSlider.value = appState.timeValue;
        updatePrice();
      }
      break;

    case "Enter":
      sendMessage("selectTime", appState.timeValue);
      break;
  }
}

// Handle keyboard input in payment state - browser stays open
function handlePaymentKeys(event) {
  if (event.key === "p" || event.key === "P") {
    // Show a loading indicator
    const statusText = document.getElementById("status-text");
    statusText.textContent = "STARTING GAME... PLEASE WAIT";

    // Send payment message - browser stays open
    sendMessage("payment", {
      gameName: appState.selectedGameName,
      minutes: appState.timeValue,
    });
    
    // Update state to game loading
    appState.currentState = STATE.GAME_LOADING;
    updateUIState(STATE.GAME_LOADING);
  }
}

// Make sure the browser is in fullscreen mode
function requestFullscreen() {
  const element = document.documentElement;

  if (element.requestFullscreen) {
    element.requestFullscreen();
  } else if (element.webkitRequestFullscreen) {
    /* Safari */
    element.webkitRequestFullscreen();
  } else if (element.msRequestFullscreen) {
    /* IE11 */
    element.msRequestFullscreen();
  } else if (element.mozRequestFullScreen) {
    /* Firefox */
    element.mozRequestFullScreen();
  }

  // Ensure we remain in fullscreen by attempting to re-enter if exited
  document.addEventListener("fullscreenchange", handleFullscreenChange);
  document.addEventListener("webkitfullscreenchange", handleFullscreenChange);
  document.addEventListener("mozfullscreenchange", handleFullscreenChange);
  document.addEventListener("MSFullscreenChange", handleFullscreenChange);
}

// Handle fullscreen change events to re-enter if needed
function handleFullscreenChange() {
  if (
    !document.fullscreenElement &&
    !document.webkitFullscreenElement &&
    !document.mozFullScreenElement &&
    !document.msFullscreenElement
  ) {
    // Attempt to re-enter fullscreen after a short delay
    setTimeout(() => {
      try {
        requestFullscreen();
      } catch (e) {
        console.error("Couldn't re-enter fullscreen mode:", e);
      }
    }, 1000);
  }
}
