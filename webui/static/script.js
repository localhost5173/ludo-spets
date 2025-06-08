// State constants
const STATE = {
  SELECT_GAME: 0,
  TIME_SELECT: 1,
  PAYMENT: 2,
  EXTEND_TIME: 3,
  EXTEND_PAYMENT: 4, // Add new state for payment confirmation after timeout
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
      updateUIState(message.payload);
      break;

    case "window_position":
      positionOverGame(message.payload);
      break;

    default:
      console.log("Unknown message type:", message.type);
  }
}

// Position the browser window over the game window
function positionOverGame(windowInfo) {
  if (appState.currentState !== STATE.EXTEND_TIME) {
    return; // Only reposition during timeout
  }

  console.log("Positioning browser over game:", windowInfo);

  // Set a unique window title that we can search for in OS commands
  document.title = "SPETS ARCADE - TIMEOUT OVERLAY";
  
  // Play a notification sound to get attention
  playAlertSound();

  // Create overlay container if it doesn't exist
  let overlay = document.getElementById("game-overlay");
  if (!overlay) {
    overlay = document.createElement("div");
    overlay.id = "game-overlay";
    document.body.appendChild(overlay);
  }

  // Style overlay to match game window - enhance styling for visibility
  overlay.style.position = "fixed";
  overlay.style.left = "0";
  overlay.style.top = "0";
  overlay.style.width = "100%";
  overlay.style.height = "100%";
  overlay.style.backgroundColor = "rgba(0, 0, 0, 0.9)"; // More opaque background
  overlay.style.display = "flex";
  overlay.style.flexDirection = "column";
  overlay.style.justifyContent = "center";
  overlay.style.alignItems = "center";
  overlay.style.zIndex = "999999"; // Extremely high z-index
  
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

  // Create timeout message with clearer two-step instructions
  let timeoutMsg = document.getElementById("timeout-message");
  if (!timeoutMsg) {
    timeoutMsg = document.createElement("h1");
    timeoutMsg.id = "timeout-message";
    timeoutMsg.textContent = "TIME OUT!";
    timeoutMsg.style.color = "#cf2e2e";
    timeoutMsg.style.fontSize = "4rem";
    timeoutMsg.style.marginBottom = "2rem";
    timeoutMsg.style.textShadow = "0 0 20px rgba(255, 0, 0, 0.7)"; // Add glow
    timeoutMsg.style.animation = "pulse 1s infinite ease-in-out"; // Add pulsing
    overlay.appendChild(timeoutMsg);
  }

  // Update instructions to emphasize ENTER key for first step
  let instructions = document.getElementById("timeout-instructions");
  if (!instructions) {
    instructions = document.createElement("p");
    instructions.id = "timeout-instructions";
    instructions.textContent = 'Step 1: Select time with ◄ ► and press ENTER to continue';
    instructions.style.color = "#fecda5";
    instructions.style.fontSize = "1.5rem";
    overlay.appendChild(instructions);
  }

  // Show the time selection UI
  const timeSelection = document.getElementById("time-selection");
  timeSelection.classList.remove("hidden");
  timeSelection.style.position = "relative";
  timeSelection.style.zIndex = "1001";

  // Add key handler for overlay - initially just for time selection
  document.addEventListener("keydown", handleTimeoutKeypress);

  // Force focus to the window
  window.focus();
  
  // Try to repeatedly focus the window for the next several seconds
  let focusAttempts = 0;
  const focusInterval = setInterval(() => {
    window.focus();
    // Flash the overlay background color to draw attention
    if (overlay) {
      const originalBg = overlay.style.backgroundColor;
      overlay.style.backgroundColor = "rgba(255, 0, 0, 0.7)";
      setTimeout(() => {
        if (overlay) overlay.style.backgroundColor = originalBg;
      }, 100);
    }
    
    focusAttempts++;
    if (focusAttempts > 20) { // Try for about 10 seconds
      clearInterval(focusInterval);
    }
  }, 500);
  
  // Add a backup timeout reminder after 30 seconds if the user hasn't interacted
  setTimeout(() => {
    if (appState.currentState === STATE.EXTEND_TIME) {
      // Flash the timeout message more aggressively
      if (timeoutMsg) {
        timeoutMsg.style.fontSize = "5rem";
        timeoutMsg.style.color = "#ff0000";
        timeoutMsg.style.animation = "pulse 0.5s infinite ease-in-out";
      }
      window.focus();
    }
  }, 30000);
}

// Handle keypresses during timeout overlay
function handleTimeoutKeypress(event) {
  if (appState.currentState !== STATE.EXTEND_TIME) {
    document.removeEventListener("keydown", handleTimeoutKeypress);
    return;
  }

  // Bring window to focus with each keypress
  window.focus();

  // Time selection handling during timeout
  switch (event.key) {
    case "ArrowLeft":
      if (appState.timeValue > 1) {
        appState.timeValue--;
        document.getElementById("time-slider").value = appState.timeValue;
        updatePrice();
      }
      break;
      
    case "ArrowRight":
      if (appState.timeValue < 60) {
        appState.timeValue++;
        document.getElementById("time-slider").value = appState.timeValue;
        updatePrice();
      }
      break;
      
    case "Enter":
      // First step: Confirm time selection and move to payment step
      const statusText = document.getElementById("status-text");
      statusText.textContent = "Step 2: PRESS 'P' TO INSERT COIN AND RESUME GAME";
      
      // Send message to confirm time selection
      sendMessage("selectTime", appState.timeValue);
      
      // Transition to payment state
      updateUIState(STATE.EXTEND_PAYMENT);
      
      // Remove this handler since we'll transition to payment state
      document.removeEventListener("keydown", handleTimeoutKeypress);
      break;
      
    case "Escape":
      // Handle quit - go back to game selection
      const overlay = document.getElementById("game-overlay");
      if (overlay) {
        overlay.style.display = "none";
      }

      // Send quit message
      sendMessage("quit", {});

      // Update UI state
      updateUIState(STATE.SELECT_GAME);

      // Remove this key handler
      document.removeEventListener("keydown", handleTimeoutKeypress);
      break;
  }
}

// Add a new handler for the payment confirmation step
function handleExtendPaymentKeys(event) {
  if (appState.currentState !== STATE.EXTEND_PAYMENT) {
    return;
  }

  if (event.key === "p" || event.key === "P") {
    // Handle payment for time extension
    const statusText = document.getElementById("status-text");
    statusText.textContent = "RESUMING GAME...";

    // Hide overlay
    const overlay = document.getElementById("game-overlay");
    if (overlay) {
      overlay.style.display = "none";
    }

    // Send payment message
    sendMessage("payment", {
      gameName: appState.selectedGameName,
      minutes: appState.timeValue,
    });

    // We'll let the window close itself after a short delay
    setTimeout(() => {
      console.log("Closing browser window after payment");
      window.close();
    }, 1000);
  }
}

// Play alert sound to grab attention
function playAlertSound() {
  try {
    // Create an audio context
    const AudioContext = window.AudioContext || window.webkitAudioContext;
    const audioCtx = new AudioContext();
    
    // Create oscillator for a beep sound
    const oscillator = audioCtx.createOscillator();
    const gainNode = audioCtx.createGain();
    
    oscillator.type = 'square';
    oscillator.frequency.value = 440; // value in hertz
    oscillator.connect(gainNode);
    gainNode.connect(audioCtx.destination);
    
    // Start and stop the beep
    oscillator.start();
    setTimeout(() => {
      oscillator.stop();
    }, 500);
  } catch (e) {
    console.error("Could not play alert sound:", e);
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
  const price = appState.timeValue * appState.pricePerMinute;
  priceLabel.textContent = `TOTAL COST: $${price.toFixed(2)} (${
    appState.timeValue
  } minutes)`;
}

// Update the UI based on the current state
function updateUIState(newState) {
  // Previous state for transitions
  const prevState = appState.currentState;
  
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

  // Remove overlay if present and not in extend time state
  if (newState !== STATE.EXTEND_TIME) {
    const overlay = document.getElementById("game-overlay");
    if (overlay) {
      overlay.style.display = "none";
    }
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
      statusText.textContent = "TIME OUT! Select additional time and press ENTER";
      timeSelection.classList.remove("hidden");
      paymentPrompt.classList.add("hidden"); // Ensure payment prompt is hidden
      updatePrice();
      break;

    case STATE.EXTEND_PAYMENT:
      statusText.textContent = "Step 2: PRESS 'P' TO INSERT COIN AND RESUME GAME";
      timeSelection.classList.add("hidden"); // Hide time selection
      paymentPrompt.classList.remove("hidden"); // Show payment prompt
      
      // Update overlay instructions if it exists
      const instructions = document.getElementById("timeout-instructions");
      if (instructions) {
        instructions.textContent = 'Step 2: Press "P" to pay and resume your game';
      }
      break;
  }
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

      case STATE.EXTEND_TIME:
        handleTimeoutKeypress(event);
        break;

      case STATE.EXTEND_PAYMENT:
        handleExtendPaymentKeys(event);
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

// Handle keyboard input in payment state
function handlePaymentKeys(event) {
  if (event.key === "p" || event.key === "P") {
    // Show a loading indicator in the status text
    const statusText = document.getElementById("status-text");
    statusText.textContent = "LAUNCHING GAME... PLEASE WAIT";

    // Send payment message and close this window
    sendMessage("payment", {
      gameName: appState.selectedGameName,
      minutes: appState.timeValue,
    });
    
    // Set a timeout to close this window after message is sent
    setTimeout(() => {
      window.close();
    }, 500);
  }
}

// Handle keyboard input in extend time state
function handleExtendTimeKeys(event) {
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

    case "p":
    case "P":
      // Show a loading indicator in the status text
      const statusText = document.getElementById("status-text");
      statusText.textContent = "RESUMING GAME... PLEASE WAIT";

      // Hide overlay if present
      const overlay = document.getElementById("game-overlay");
      if (overlay) {
        overlay.style.display = "none";
      }

      // Send payment message
      sendMessage("payment", {
        gameName: appState.selectedGameName,
        minutes: appState.timeValue,
      });
      
      // Set a timeout to close this window after message is sent
      setTimeout(() => {
        window.close();
      }, 500);
      break;
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
