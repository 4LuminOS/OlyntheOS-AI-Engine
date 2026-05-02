cmake_minimum_required(VERSION 3.16)
project(lumin-control-panel)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTORCC ON)
set(CMAKE_AUTOUIC ON)

find_package(Qt6 COMPONENTS
    Core
    Gui
    Qml
    Quick
    Network
    REQUIRED
)

find_package(KF6 COMPONENTS
    Kirigami
    REQUIRED
)

set(SOURCES
    src/main.cpp
    resources/qml.qrc
)

add_executable(${PROJECT_NAME} ${SOURCES})

target_link_libraries(${PROJECT_NAME}
    Qt6::Core
    Qt6::Gui
    Qt6::Qml
    Qt6::Quick
    Qt6::Network
    KF6::Kirigami
)

# Install targets
install(TARGETS ${PROJECT_NAME} DESTINATION bin)
install(FILES main.qml DESTINATION share/lumin-control-panel)
install(FILES IPCClient.qml DESTINATION share/lumin-control-panel)
