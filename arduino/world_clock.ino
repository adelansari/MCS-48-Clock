#define PERIOD 2
#define PIN 13
#define INTERVAL PERIOD * 2

void setup() {
  pinMode(PIN, OUTPUT);
  digitalWrite(PIN, LOW);
  Serial.begin(19200);
  // Delay to make sure the clock registers the low state
  delay(PERIOD * 2);
}

void loop() {
  if (Serial.available()) {
    char c = Serial.read();
    serialParse(c);
  }
}

void loop_demo() {
  delay(1000);
  send_12_34();
  delay(1000);
  for (int i = 0; i < 120; i++) {
    send_increment();
    delay(1000);
  }
}

void serialParse(char c) {
  byte s;
  if (c == 'S') {
    send_start();
    return;
  } else if (c >= '0' && c <= '9') {
    s = c - '0';
  } else if (c >= 'A' && c <= 'F') {
    s = c - 'A' + 10;
  } else if (c == '\n' || c == '\r') {
     return;
  } else {
    Serial.print("\nSerial parse error\n");
    return;
  }
  send_nibble(s);
}

void send_12_34() {
  for (int i = 0; i < 9; i++) {
    send_start();
    send_nibble(i);
    send_nibble(4);
    send_nibble(3);
    send_nibble(i);
    send_nibble(1);
    delay(PERIOD * 2);
  }
  
}

void send_nibble(uint8_t n) {
  for (int i = 0; i < 4; i++) {
    if (n & 1 == 1) {
      send_1();
    } else {
      send_0();
    }
    n = n >> 1;
  }
}


void send_increment() {
  send_start();
  send_nibble(B11110);
}

void send_all_clocks() {
  for (int i = 0; i < 9; i++) {
    send_start();
    for (int j = 0; j < 5; j++) {
      send_nibble(i);
    }
    delay(PERIOD * 2);
  }
}

void send_0() {
  Serial.print(0);
  send(2);
}

void send_1() {
  Serial.print(1);
  send(5);
}

void send_start() {
  Serial.print("\n2");
  send(8);
}

void send(int t) {
  digitalWrite(PIN, HIGH);
  delay(PERIOD * t);
  digitalWrite(PIN, LOW);
  delay(INTERVAL);
}
