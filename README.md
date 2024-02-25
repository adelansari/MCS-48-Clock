# MCS-48-Clock

Reverse engineering and grokking the MCS-48 System of wharton world clock

This is a 4 timezone clock that has been manufactured on may 1993. It arrived without any documentation or the central clock to drive it.

The clock receives its time by one pair of signal wires, that go to an optocoupler. The display is controlled by a NEC uPD8749 microcontroller. It has 2kb of eprom program storage and 128 bytes of ram.

The aim was to dump the firmware and reverse engineered it in ghidra to figure out the communication protocol.

## Hardware level of the control signal

The signal should work on voltage levels between 10 and 30 volts. The receiver has an ILD74 optcoupler with a 1k ohm series resistor on it.

## Timing

The signal is really slow, with sampling occuring every 2 milliseconds. The data is encoded by the length of high pulses on the line. Symbols are separated by one or more sampled low states, their number doesn't matter.

Following symbols are used, depending on the length of the sampled high pulses:

- 0 - 1-3 sampled high states, 2-6ms duration
- 1 - 4-6 sampled high states, 8-12ms duration
- start marker - 7-9 sampled high states, 14-18ms duration

Longer pulses are treated as error and cause the clock display to blank.

## Messages

There are two types of messages supported by the firmware, one to advance all clocks by one minute and one to set an addressed clock to given time.

All messages start with the start marker, but transmitting the start marker in the middle of a message doesn't reset the receive logic, only makes
the message decoding produce unwanted values.

### Increment message

Message format: `<start marker> 0 1 1 1` or `<start> 0xE` for little endian nibble notation. Total of start + 4 payload bits

### Set time message

Message format: `<start marker> <4bit address> 4*<BCD digit>`. Total of start + 20 payload bits.

- The address specifies the destination clock, address 0xE is reserved for the increment command. Most clocks seem to use 3bit address.
- The time is sent in binary coded decimal (BCD) format, with 4 bits per digit, starting from the least significant digit.
  - First digit sent is the low digit of minutes
  - Followed by the high digit of minutes
  - Low digit of hours
  - high digit of hours

## Credit

All rights for the firmware dump and material goes to [depili](https://github.com/depili). I attended a workshop he was organizing and learned a lot by working on the code on ghidra.
