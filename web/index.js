import { h, render, Component } from 'preact'  // eslint-disable-line
import makeAccumulator from 'sorted-immutable-list'
import humanDate from 'tiny-human-time'
import Linkify from 'react-linkify'
import sanitizeHtml from 'sanitize-html-react'
import Parse from 'react-html-parser'
import AutoScroll from 'react-auto-scroll'
import ColorAssigner from '@convergence/color-assigner'
import {
  Comment,
  Form,
  Header,
  Input,
  Menu
} from 'semantic-ui-react'

import {HotKeys} from 'react-hotkeys'

var color = new ColorAssigner()

var ws = new window.WebSocket('ws://localhost:8001/ws')

const addMessage = makeAccumulator({
  key: msg => msg.Timestamp,
  unique: false
})

var state = {
  messages: [],
  activeChannel: {},
  channels: {},
  users: {}
}

ws.addEventListener('open', function () {
  sendCommand('get_channels')
  sendCommand('get_users')
})

ws.addEventListener('message', function (msg, flags) {
  const data = JSON.parse(msg.data)

  if (data.Type === 'message') {
    updateState(Object.assign({}, state, {
      messages: addMessage(
        state.messages,
        Object.assign(
          {},
          data.Message,
          { niceTime: humanDate(new Date(data.Message.Timestamp), new Date()) }))
    }))
  } else if (data.Type === 'user') {
    updateState(
      Object.assign(
        {},
        state,
        {
          users: Object.assign(
            {},
            state.users, {
              [data.User.ID]: data.User
            }
          )
        }
      )
    )
  } else if (data.Type === 'channel') {
    updateState(
      Object.assign(
        {},
        state,
        {
          channels: Object.assign(
          {},
          state.channels, {
            [data.Channel.ID]: data.Channel
          }
          )
        }
      )
    )
  }
})

ws.addEventListener('close', function () {
  ws = new window.WebSocket('ws://localhost:8001/ws')
})

var stateThrottle

function updateState (newState) {
  window.state = state = newState
  clearTimeout(stateThrottle)
  stateThrottle = setTimeout(() => redraw(state), 100)
}

function setActiveChannel (channel) {
  if (channel.ID === state.activeChannel.ID) return
  updateState(Object.assign({}, state, {
    activeChannel: channel
  }))
  var existingMessages = getMessagesInChannel(state.messages, channel)
  if (existingMessages.length < 100) {
    var commandParams = {
      Channel: channel.ID
    }
    if (existingMessages.length) {
      commandParams.Offset = existingMessages[0].Timestamp
    }
    sendCommand('replay_messages', commandParams)
  }
}

function sendCommand (Command, Params) {
  ws.send(JSON.stringify({
    Type: 'command',
    Message: {
      Type: Command,
      Command: Object.assign({}, Params)
    }
  }))
}

function sendMessage ({Account, Channel, User}, text) {
  ws.send(JSON.stringify({
    Type: 'message',
    Message: {
      Channel: Channel || User,
      To: Account,
      Text: text
    }
  }))
}

function getMessagesInChannel (messages, channel) {
  return messages.filter(
    m => {
      return (
        m.Channel === channel.Channel ||
        m.Channel === channel.User
      ) && (
        m.Account === channel.Account ||
        m.To === channel.Account
      )
    }
  )
}

function groupSameUsers (messages) {
  var lastUser
  return messages.reduce((grouppedMessages, m) => {
    if (lastUser === m.Username) {
      const lastMsg = Object.assign({}, grouppedMessages.pop())
      lastMsg.Text += '\n' + m.Text
      m = lastMsg
    }
    lastUser = m.Username
    return grouppedMessages.concat([m])
  }, [])
}

function nicerText (text) {
  return text.replace(/(\n)+/g, '\n')
}

class Feed extends Component {
  render () {
    return <Comment.Group minimal className='feed'>
      {
        groupSameUsers(getMessagesInChannel(
          this.props.messages, this.props.activeChannel)
        ).map((m) => {
          const user = this.props.users[`${m.Username}:${m.Account}`]
          const userName = user && user.Name || m.Username
          return <Comment>
            <Comment.Content>
              <Comment.Author as='a' style={{ color: color.getColorAsRgba(userName) }}>
                { userName }
              </Comment.Author>
              <Comment.Metadata>
                <span>{ m.niceTime }</span>
              </Comment.Metadata>
              <Comment.Text>
                <Linkify>{ Parse(sanitizeHtml(nicerText(m.Text))) }</Linkify>
              </Comment.Text>
            </Comment.Content>
          </Comment>
        })
      }
    </Comment.Group>
  }
}

const AutoScrollFeed = AutoScroll({
  property: 'messages',
  alwaysScroll: true
})(Feed)

class Channels extends Component {

  constructor () {
    super()
    this.state.filter = ''
  }

  focusChannelSearch () {
    this.channelSearch.focus()
    this.channelSearch.select()
  }

  searchChannel (e) {
    const filter = e.target.value
    this.setState({filter: filter})
    const newActiveChannel = this.props.channels.filter(c => {
      return !this.state.filter || c.ID.indexOf(this.state.filter) >= 0
    })[0]

    if (!newActiveChannel) return
    setActiveChannel(newActiveChannel)
  }

  render ({channels, activeChannel, visible}) {
    return <div>
      <Menu.Item>
        <Input placeholder='Search...' >
          <input
            type='text'
            ref={(el) => { this.channelSearch = el }}
            onKeyup={(e) => this.searchChannel(e)}
          />
        </Input>
      </Menu.Item>
      {
        channels.filter(c => {
          return c.ID && (!this.state.filter || c.ID.indexOf(this.state.filter) >= 0)
        }).map(c => (
          <Menu.Item onClick={() => setActiveChannel(c)} active={c === activeChannel} >
            <span style={{
              maxWidth: '140px',
              overflow: 'hidden',
              display: 'inline-block' }}>
              { c.Name || c.ID }
            </span>
          </Menu.Item>
        ))
      }
    </div>
  }
}

const MessageInput = ({activeChannel}) => {
  return <Form onSubmit={(e, data) => {
    e.preventDefault()
    sendMessage(activeChannel, data.formData.msg)
    e.target.firstChild.firstChild.value = ''
  }}>
    <Input fluid name='msg' action='Send' placeholder='Type in something...' />
  </Form>
}

const keyMap = {
  'focusChannelSearch': 'alt+k'
}

class App extends Component {
  render ({state}) {
    return <HotKeys
      keyMap={keyMap}
      style={{height: '100%'}}
      handlers={{
        'focusChannelSearch': () => this.channels.focusChannelSearch()
      }}>
      <div class='content'>
        <div class='chat-sidebar ui vertical inverted menu'>
          <Channels
            ref={(el) => { this.channels = el }}
            visible
            channels={Object.values(state.channels).concat(Object.values(state.users))}
            activeChannel={state.activeChannel} />
        </div>
        <div class='chat'>
          <Header as='h2' content={state.activeChannel.Name} />
          <AutoScrollFeed messages={state.messages} users={state.users} activeChannel={state.activeChannel} />
          <MessageInput activeChannel={state.activeChannel} />
        </div>
      </div>
    </HotKeys>
  }
}

function redraw (newState) {
  const chat = document.querySelector('#chat')
  chat.style.height = '100%'
  render((<App state={newState} />), chat, chat.lastChild)
}

redraw(state)
